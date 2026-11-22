// Package cue spots what the other side of a conversation is asking about and suggests the
// script section that answers it (docs/requirements/07-stage2-cue-suggestions.md).
package cue

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jose00735/telecyborg/internal/script"
	"github.com/jose00735/telecyborg/internal/words"
)

// Suggestion is a section worth jumping to.
type Suggestion struct {
	Sec     int      `json:"sec"`
	Title   string   `json:"title"`
	QA      bool     `json:"qa"`
	Score   float64  `json:"score"`
	Matched []string `json:"matched"`
}

type phrase struct {
	label  string     // shown to the user
	alts   [][]string // alternatives, each a list of stemmed tokens
	weight float64
}

type target struct {
	sec     int
	title   string
	qa      bool
	phrases []phrase
}

type heard struct {
	stem  string
	at    time.Time
	batch int
}

// Matcher keeps a rolling window of what was heard and scores sections against it.
type Matcher struct {
	mu        sync.Mutex
	targets   []target
	heard     []heard
	batch     int
	questionT time.Time
	// Window is how long heard words count (default 25 s); HalfLife is the recency decay.
	Window    time.Duration
	HalfLife  time.Duration
	Threshold float64
}

// Synonym groups (folded). A keyword in a group also matches the others.
var synonymGroups = [][]string{
	{"price", "pricing", "cost", "precio", "costo", "preco", "custo"},
	{"salary", "compensation", "pay", "salario", "sueldo", "remuneracion", "aspiracion salarial", "pretension salarial", "pretensao salarial"},
	{"weakness", "weaknesses", "debilidad", "debilidades", "fraqueza", "fraquezas", "area of improvement"},
	{"strength", "strengths", "fortaleza", "fortalezas", "ponto forte", "pontos fortes"},
	{"customer", "client", "cliente", "clientes", "account"},
	{"team", "equipo", "equipe"},
	{"challenge", "reto", "desafio", "challenging", "difficult"},
	{"conflict", "conflicto", "conflito", "disagreement", "desacuerdo"},
	{"experience", "experiencia", "background", "trayectoria"},
	{"failure", "fail", "failed", "fracaso", "falla", "error", "mistake", "erro"},
	{"why", "por que"},
	{"goal", "goals", "meta", "metas", "objetivo", "objetivos", "five years", "cinco anos"},
	{"renewal", "renew", "renovacion", "renovacao", "churn"},
	{"security", "seguridad", "seguranca", "cybersecurity", "ciberseguridad"},
	{"migration", "migracion", "migracao", "migrate"},
	{"integration", "integracion", "integracao", "integrate"},
	{"roadmap", "hoja de ruta"},
	{"support", "soporte", "suporte", "troubleshooting"},
	{"leadership", "liderazgo", "lideranca", "lead", "led"},
}

var synIndex = map[string]int{}

func init() {
	for i, g := range synonymGroups {
		for _, w := range g {
			synIndex[strings.Join(words.StemAll(words.Tokens(w)), " ")] = i
		}
	}
}

func stemPhrase(s string) []string {
	out := []string{}
	for _, t := range words.Tokens(s) {
		if !words.IsStop(t) || len(words.Tokens(s)) > 1 {
			out = append(out, words.Stem(t))
		}
	}
	return out
}

func makePhrase(label string, weight float64) (phrase, bool) {
	p := phrase{label: strings.TrimSpace(label), weight: weight}
	for _, alt := range strings.Split(label, "/") {
		toks := stemPhrase(alt)
		if len(toks) == 0 {
			continue
		}
		p.alts = append(p.alts, toks)
		if g, ok := synIndex[strings.Join(toks, " ")]; ok {
			for _, w := range synonymGroups[g] {
				if st := words.StemAll(words.Tokens(w)); len(st) > 0 {
					p.alts = append(p.alts, st)
				}
			}
		}
	}
	return p, len(p.alts) > 0
}

// New builds a matcher from a parsed script.
func New(p script.Parsed) *Matcher {
	m := &Matcher{Window: 25 * time.Second, HalfLife: 10 * time.Second, Threshold: 1.0}
	for _, s := range p.Outline {
		t := target{sec: s.Index, title: s.Title, qa: s.QA}
		for _, k := range s.Keywords {
			w := 1.0
			if len(words.Tokens(k)) > 1 {
				w = 1.4
			}
			if ph, ok := makePhrase(k, w); ok {
				t.phrases = append(t.phrases, ph)
			}
		}
		// automatic keywords from the title
		title := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(s.Title, "Q:"), "P:"))
		content := []string{}
		for _, tok := range words.Tokens(title) {
			if !words.IsStop(tok) && len(tok) > 2 {
				content = append(content, tok)
			}
		}
		for _, tok := range content {
			if ph, ok := makePhrase(tok, 0.5); ok {
				t.phrases = append(t.phrases, ph)
			}
		}
		if len(content) >= 2 {
			if ph, ok := makePhrase(strings.Join(content, " "), 0.8); ok {
				t.phrases = append(t.phrases, ph)
			}
		}
		m.targets = append(m.targets, t)
	}
	// a phrase that appears in many sections says little: divide by sqrt(sections using it)
	count := map[string]int{}
	for _, t := range m.targets {
		seen := map[string]bool{}
		for _, ph := range t.phrases {
			k := strings.Join(ph.alts[0], " ")
			if !seen[k] {
				count[k]++
				seen[k] = true
			}
		}
	}
	for i := range m.targets {
		for j := range m.targets[i].phrases {
			ph := &m.targets[i].phrases[j]
			if n := count[strings.Join(ph.alts[0], " ")]; n > 1 {
				ph.weight /= math.Sqrt(float64(n))
			}
		}
	}
	return m
}

var questionRe = regexp.MustCompile(`(?i)\?|\b(tell me|walk me|how (do|did|would|will)|what (is|are|was|would|do|did)|why|could you|can you|describe|explain|cu[eé]ntame|h[aá]blame|c[oó]mo|qu[eé]|por qu[eé]|cu[aá]l|me cuentas|por que|o que|como voc[eê]|qual|fale)\b`)

// Hear adds a transcript chunk and returns the sections it points to, best first (at most 3).
// Only sections with evidence in this chunk are returned, so a suggestion is not repeated
// while the same words decay.
func (m *Matcher) Hear(text string, at time.Time) []Suggestion {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.batch++
	for _, t := range words.Tokens(text) {
		m.heard = append(m.heard, heard{stem: words.Stem(t), at: at, batch: m.batch})
	}
	if questionRe.MatchString(text) {
		m.questionT = at
	}
	cut := at.Add(-m.Window)
	k := 0
	for k < len(m.heard) && m.heard[k].at.Before(cut) {
		k++
	}
	m.heard = m.heard[k:]

	var out []Suggestion
	for _, t := range m.targets {
		score, fresh := 0.0, false
		var matched []string
		for _, ph := range t.phrases {
			end, ok := m.find(ph)
			if !ok {
				continue
			}
			h := m.heard[end]
			age := at.Sub(h.at).Seconds()
			score += ph.weight * math.Pow(0.5, age/m.HalfLife.Seconds())
			if h.batch == m.batch {
				fresh = true
			}
			matched = append(matched, ph.label)
		}
		if t.qa && !m.questionT.IsZero() && at.Sub(m.questionT) < 12*time.Second {
			score *= 1.3
		}
		if fresh && score >= m.Threshold {
			out = append(out, Suggestion{Sec: t.sec, Title: t.title, QA: t.qa, Score: math.Round(score*100) / 100, Matched: dedupe(matched)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Sec < out[j].Sec
	})
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

// find returns the index in m.heard where the latest occurrence of the phrase ends.
// Multi-word phrases may have one extra word between their words ("pricing of the model").
func (m *Matcher) find(ph phrase) (int, bool) {
	best := -1
	for _, alt := range ph.alts {
		for end := len(m.heard) - 1; end >= 0 && end > best; end-- {
			if !words.Close(m.heard[end].stem, alt[len(alt)-1]) {
				continue
			}
			i, j, gaps := end-1, len(alt)-2, 0
			for j >= 0 && i >= 0 {
				if words.Close(m.heard[i].stem, alt[j]) {
					j--
				} else if gaps++; gaps > len(alt) {
					break
				}
				i--
			}
			if j < 0 {
				best = end
				break
			}
		}
	}
	return best, best >= 0
}

// Reset forgets what was heard.
func (m *Matcher) Reset() {
	m.mu.Lock()
	m.heard = nil
	m.mu.Unlock()
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if !seen[strings.ToLower(s)] {
			seen[strings.ToLower(s)] = true
			out = append(out, s)
		}
	}
	return out
}
