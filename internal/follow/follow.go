// Package follow aligns what the speaker says with the script, so the prompter can scroll with
// the voice instead of a clock (docs/requirements/09-stage3-smart-follow.md).
package follow

import (
	"strings"
	"sync"

	"github.com/jose00735/telecyborg/internal/script"
	"github.com/jose00735/telecyborg/internal/words"
)

type word struct {
	tok    string
	para   int // paragraph index within the track
	inPara int
}

// Position is where the speaker is: Word is the index of the next word to be said.
type Position struct {
	Word       int     `json:"word"`
	Total      int     `json:"total"`
	Para       int     `json:"para"`
	InPara     int     `json:"inPara"`
	ParaWords  int     `json:"paraWords"`
	Accepted   bool    `json:"accepted"`   // the last speech matched the script
	Moved      bool    `json:"moved"`      // the position advanced
	Confidence float64 `json:"confidence"` // 0..1 for the accepted match
}

// Aligner tracks one track of paragraphs (the main flow or one Q&A answer).
type Aligner struct {
	mu        sync.Mutex
	words     []word
	paraWords []int
	pos       int
	spoken    []string
	// Lookahead is how far ahead (in words) a normal match may land (default 60).
	Lookahead int
}

// New builds an aligner over paragraph texts, in reading order. Tokens line up one to one with
// script.CountWords, so positions map onto the prompter's per-paragraph word counts.
func New(paras []string) *Aligner {
	a := &Aligner{Lookahead: 60}
	for pi, text := range paras {
		ws := script.Words(text)
		a.paraWords = append(a.paraWords, len(ws))
		for wi, w := range ws {
			a.words = append(a.words, word{tok: strings.ReplaceAll(words.Fold(w), " ", ""), para: pi, inPara: wi})
		}
	}
	return a
}

// ForTrack returns the paragraph texts of a parsed script for a track: sec < 0 is the main flow,
// otherwise the paragraphs of that section.
func ForTrack(p script.Parsed, sec int) []string {
	out := []string{}
	for _, b := range p.Blocks {
		if b.Kind != "para" {
			continue
		}
		if (sec < 0 && !b.QA) || (sec >= 0 && b.Sec == sec) {
			out = append(out, b.Text)
		}
	}
	return out
}

var hesitation = map[string]bool{"um": true, "umm": true, "uh": true, "uhm": true, "eh": true, "ehh": true, "erm": true, "hmm": true, "mm": true, "mmm": true, "ah": true}

// Seek moves to paragraph para (after a manual jump).
func (a *Aligner) Seek(para int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.spoken = nil
	for i, w := range a.words {
		if w.para >= para {
			a.pos = i
			return
		}
	}
	a.pos = len(a.words)
}

func (a *Aligner) position(accepted, moved bool, conf float64) Position {
	p := Position{Word: a.pos, Total: len(a.words), Accepted: accepted, Moved: moved, Confidence: conf}
	if a.pos < len(a.words) {
		w := a.words[a.pos]
		p.Para, p.InPara, p.ParaWords = w.para, w.inPara, a.paraWords[w.para]
	} else if len(a.paraWords) > 0 {
		p.Para = len(a.paraWords) - 1
		p.ParaWords = a.paraWords[p.Para]
		p.InPara = p.ParaWords
	}
	return p
}

// Current returns the position without new speech.
func (a *Aligner) Current() Position {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.position(false, false, 0)
}

// Hear aligns a new piece of transcript and returns the updated position. The position only
// moves forward: repeated or re-transcribed words never scroll the script back.
func (a *Aligner) Hear(text string) Position {
	a.mu.Lock()
	defer a.mu.Unlock()
	var nt []string
	for _, t := range words.Tokens(text) {
		if !hesitation[t] {
			nt = append(nt, t)
		}
	}
	// audio windows overlap: drop the start of this chunk when it repeats the end of the last one
	for m := min(len(nt), len(a.spoken), 12); m >= 2; m-- {
		same := true
		for i := 0; i < m; i++ {
			if !words.Close(a.spoken[len(a.spoken)-m+i], nt[i]) {
				same = false
				break
			}
		}
		if same {
			nt = nt[m:]
			break
		}
	}
	a.spoken = append(a.spoken, nt...)
	if len(a.spoken) > 24 {
		a.spoken = a.spoken[len(a.spoken)-24:]
	}
	n := len(a.words)
	if n == 0 || len(a.spoken) == 0 {
		return a.position(false, false, 0)
	}
	k := len(a.spoken)
	if k > 10 {
		k = 10
	}
	tail := a.spoken[len(a.spoken)-k:]
	minL := 3
	if k < 3 {
		minL = k
	}

	best, bestL, bestScore := -1, 0, 0.0
	try := func(from, to int, need float64, needL int) {
		for e := from; e < to; e++ {
			l, trailing := a.lcsEnding(tail, e)
			if l < needL {
				continue
			}
			// spoken words after the match point should have matched script words after e
			sc := (float64(l) - 0.5*float64(trailing)) / float64(k)
			if sc < need {
				continue
			}
			// prefer higher score; on a tie prefer the candidate nearest the expected position
			if sc > bestScore+1e-9 || (sc > bestScore-1e-9 && dist(e, a.pos) < dist(best, a.pos)) {
				best, bestL, bestScore = e, l, sc
			}
		}
	}
	lo := a.pos - 8
	if lo < 0 {
		lo = 0
	}
	hi := a.pos + a.Lookahead
	if hi > n {
		hi = n
	}
	try(lo, hi, 0.5, minL)
	if best >= 0 && best+1-a.pos > 30 && (bestScore < 0.7 || bestL < 5) {
		best = -1 // a long jump needs strong evidence
	}
	if best < 0 && len(a.spoken) >= 6 {
		// the speaker skipped ahead beyond the window: search the rest of the track, strictly
		bestScore = 0
		try(hi, n, 0.75, 6)
	}
	if best < 0 {
		return a.position(false, false, 0)
	}
	moved := false
	if best+1 > a.pos {
		a.pos = best + 1
		moved = true
	}
	return a.position(true, moved, bestScore)
}

func dist(a, b int) int {
	if a < 0 {
		return 1 << 30
	}
	if a > b {
		return a - b
	}
	return (b - a) * 2 // being behind is less likely than being slightly ahead
}

// lcsEnding is the longest common subsequence between tail and the script window ending at e,
// where script word e must be part of the match, and how many spoken words come after the
// spoken word matched to e.
func (a *Aligner) lcsEnding(tail []string, e int) (int, int) {
	if !matchesAny(a.words[e].tok, tail) {
		return 0, 0
	}
	start := e - 2*len(tail)
	if start < 0 {
		start = 0
	}
	win := a.words[start : e+1]
	// dp over tail x win; require the last window word to be matched by reading dp at the end
	m, w := len(tail), len(win)
	prev := make([]int, w+1)
	cur := make([]int, w+1)
	bestEnd, bestI := 0, 0
	for i := 1; i <= m; i++ {
		for j := 1; j <= w; j++ {
			if words.Close(tail[i-1], win[j-1].tok) {
				cur[j] = prev[j-1] + 1
			} else if prev[j] >= cur[j-1] {
				cur[j] = prev[j]
			} else {
				cur[j] = cur[j-1]
			}
		}
		// best alignment where tail[i-1] is matched to the last window word
		if words.Close(tail[i-1], win[w-1].tok) && prev[w-1]+1 >= bestEnd {
			bestEnd, bestI = prev[w-1]+1, i
		}
		prev, cur = cur, prev
	}
	return bestEnd, m - bestI
}

func matchesAny(tok string, tail []string) bool {
	for _, t := range tail {
		if words.Close(t, tok) {
			return true
		}
	}
	return false
}
