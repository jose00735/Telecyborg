// Package live holds a listening session: what the other side said (for cue suggestions),
// what the speaker said (for smart follow) and the post-meeting notes. Transcripts live in
// memory and are written to disk only when the user turns on "save transcript" (PV-3).
package live

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jose00735/telecyborg/internal/cue"
	"github.com/jose00735/telecyborg/internal/follow"
	"github.com/jose00735/telecyborg/internal/script"
	"github.com/jose00735/telecyborg/internal/words"
)

// Line is one transcribed chunk.
type Line struct {
	At   float64 `json:"at"`   // seconds since the session started
	Role string  `json:"role"` // "other" or "me"
	Text string  `json:"text"`
}

// Session is one listening session over one script.
type Session struct {
	ID       string
	ScriptID string
	Title    string
	Save     bool
	Started  time.Time
	Parsed   script.Parsed

	mu      sync.Mutex
	matcher *cue.Matcher
	tracks  map[int]*follow.Aligner // -1 = main flow, otherwise a section index
	lines   []Line
	fillers map[string]int
	heardMe int
}

// Result is what the UI gets back for a chunk.
type Result struct {
	Role        string           `json:"role"`
	Text        string           `json:"text"`
	Suggestions []cue.Suggestion `json:"suggestions,omitempty"`
	Position    *follow.Position `json:"position,omitempty"`
	Track       int              `json:"track"`
	Fillers     map[string]int   `json:"fillers,omitempty"` // in this chunk
}

// Registry keeps the open sessions.
type Registry struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry { return &Registry{sessions: map[string]*Session{}} }

// Start opens a session.
func (r *Registry) Start(scriptID, title string, p script.Parsed, save bool) *Session {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	s := &Session{ID: hex.EncodeToString(b), ScriptID: scriptID, Title: title, Save: save, Started: time.Now(), Parsed: p,
		matcher: cue.New(p), tracks: map[int]*follow.Aligner{}, fillers: map[string]int{}}
	r.mu.Lock()
	defer r.mu.Unlock()
	// one live session at a time: a new rehearsal replaces the old one
	for id := range r.sessions {
		delete(r.sessions, id)
	}
	r.sessions[s.ID] = s
	return s
}

// Get returns a session.
func (r *Registry) Get(id string) *Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessions[id]
}

// End removes a session.
func (r *Registry) End(id string) {
	r.mu.Lock()
	delete(r.sessions, id)
	r.mu.Unlock()
}

func (s *Session) track(t int) *follow.Aligner {
	a, ok := s.tracks[t]
	if !ok {
		a = follow.New(follow.ForTrack(s.Parsed, t))
		s.tracks[t] = a
	}
	return a
}

// Hear processes a transcript chunk from one side.
func (s *Session) Hear(role, text string, track int) Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := Result{Role: role, Text: text, Track: track}
	if strings.TrimSpace(text) == "" {
		return res
	}
	now := time.Now()
	s.lines = append(s.lines, Line{At: now.Sub(s.Started).Seconds(), Role: role, Text: text})
	if role == "other" {
		res.Suggestions = s.matcher.Hear(text, now)
		return res
	}
	pos := s.track(track).Hear(text)
	res.Position = &pos
	if f := words.CountFillers(text); len(f) > 0 {
		res.Fillers = f
		for k, v := range f {
			s.fillers[k] += v
		}
	}
	return res
}

// Seek tells the aligner of a track that the speaker jumped to a paragraph.
func (s *Session) Seek(track, para int) follow.Position {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.track(track)
	a.Seek(para)
	return a.Current()
}

// PromptFor returns the whisper prompt for a side: the upcoming script words for the speaker
// (so names and jargon are spelled like the script) and the cue keywords for the other side.
func (s *Session) PromptFor(role string, track int, lang string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	lead := map[string]string{"es": "Eh, bueno, este... ", "pt": "Hum, então, né... "}[lang]
	if lead == "" {
		lead = "Um, so, uh... "
	}
	if role == "other" {
		var kw []string
		for _, o := range s.Parsed.Outline {
			kw = append(kw, o.Keywords...)
		}
		p := strings.Join(kw, ", ")
		if len(p) > 300 {
			p = p[:300]
		}
		return p
	}
	a := s.track(track)
	pos := a.Current()
	paras := follow.ForTrack(s.Parsed, track)
	var b strings.Builder
	for i := pos.Para; i < len(paras) && b.Len() < 360; i++ {
		ws := script.Words(paras[i])
		from := 0
		if i == pos.Para {
			from = pos.InPara - 12
			if from < 0 {
				from = 0
			}
		}
		if from < len(ws) {
			b.WriteString(strings.Join(ws[from:], " ") + " ")
		}
	}
	return lead + strings.TrimSpace(b.String())
}

// Fillers returns the filler counts of the session.
func (s *Session) Fillers() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]int{}
	for k, v := range s.fillers {
		out[k] = v
	}
	return out
}

var askRe = regexp.MustCompile(`(?i)\?|^(tell me|walk me|how|what|why|could you|can you|describe|explain|cu[eé]ntame|h[aá]blame|c[oó]mo|qu[eé]|por qu[eé]|cu[aá]l|o que|por que|qual|fale)\b`)

// Notes builds the post-meeting notes: the questions you were asked, the sections you used
// and, when saving is on, the transcript.
func (s *Session) Notes(used []string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b strings.Builder
	fmt.Fprintf(&b, "# Notes — %s\n\n%s · %s\n\n", s.Title, s.Started.Format("2006-01-02 15:04"), fmtDur(time.Since(s.Started).Seconds()))
	b.WriteString("## Questions you were asked\n")
	n := 0
	for _, l := range s.lines {
		if l.Role != "other" {
			continue
		}
		for _, sent := range splitSentences(l.Text) {
			if askRe.MatchString(sent) {
				fmt.Fprintf(&b, "- %s _(at %s)_\n", sent, fmtDur(l.At))
				n++
			}
		}
	}
	if n == 0 {
		b.WriteString("- none detected\n")
	}
	b.WriteString("\n## Sections you used\n")
	if len(used) == 0 {
		b.WriteString("- only the main narrative\n")
	}
	for _, u := range used {
		fmt.Fprintf(&b, "- %s\n", u)
	}
	if len(s.fillers) > 0 {
		b.WriteString("\n## Filler words\n")
		keys := make([]string, 0, len(s.fillers))
		for k := range s.fillers {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return s.fillers[keys[i]] > s.fillers[keys[j]] })
		for _, k := range keys {
			fmt.Fprintf(&b, "- %s: %d\n", k, s.fillers[k])
		}
	}
	if s.Save {
		b.WriteString("\n## Transcript\n")
		for _, l := range s.lines {
			who := "Them"
			if l.Role == "me" {
				who = "Me"
			}
			fmt.Fprintf(&b, "- **%s** %s — %s\n", fmtDur(l.At), who, l.Text)
		}
	}
	return b.String()
}

// SaveNotes writes the notes into dir/transcripts when the session has saving on.
func (s *Session) SaveNotes(dir, notes string) (string, error) {
	if !s.Save {
		return "", nil
	}
	d := filepath.Join(dir, "transcripts")
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(d, s.ScriptID+"-"+s.Started.Format("20060102-1504")+".md")
	return p, os.WriteFile(p, []byte(notes), 0o644)
}

func splitSentences(t string) []string {
	var out []string
	cur := strings.Builder{}
	for _, r := range t {
		cur.WriteRune(r)
		if r == '?' || r == '.' || r == '!' {
			if s := strings.TrimSpace(cur.String()); s != "" {
				out = append(out, s)
			}
			cur.Reset()
		}
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		out = append(out, s)
	}
	return out
}

func fmtDur(sec float64) string {
	t := int(sec + 0.5)
	return fmt.Sprintf("%d:%02d", t/60, t%60)
}
