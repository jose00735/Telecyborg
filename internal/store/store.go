// Package store keeps scripts, settings and rehearsal history as plain files.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/jose00735/telecyborg/internal/script"
)

var ErrNotFound = errors.New("not found")

// Store is a directory with scripts/, rehearsals/ and settings.json.
type Store struct {
	Dir string
	mu  sync.Mutex
}

// Summary describes a script in the library.
type Summary struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Words      int     `json:"words"`
	Paragraphs int     `json:"paragraphs"`
	WPM        int     `json:"wpm"`
	Target     float64 `json:"target"`
	Duration   float64 `json:"duration"`
	Modified   string  `json:"modified"`
	Runs       int     `json:"runs"`
}

// Script is a script with its source and parsed form.
type Script struct {
	Summary
	Source string        `json:"source"`
	Body   string        `json:"body"`
	Parsed script.Parsed `json:"parsed"`
}

// Settings are per-user preferences.
type Settings struct {
	WPM       int     `json:"wpm"`
	FontSize  int     `json:"fontSize"`
	Mirror    bool    `json:"mirror"`
	FocusLine float64 `json:"focusLine"` // 0.2 - 0.6 of window height
	OnTop     bool    `json:"onTop"`
	Countdown int     `json:"countdown"`
	SeenIntro bool    `json:"seenIntro"`
	Motion    string  `json:"motion"` // "full" or "reduced"
}

// DefaultSettings for a first launch.
func DefaultSettings() Settings {
	return Settings{WPM: script.DefaultWPM, FontSize: 44, FocusLine: 0.38, Countdown: 3, Motion: "full"}
}

// Rehearsal is one run of a script.
type Rehearsal struct {
	At        string    `json:"at"`
	Planned   float64   `json:"planned"`
	Actual    float64   `json:"actual"`
	Target    float64   `json:"target"`
	WPMStart  int       `json:"wpmStart"`
	WPMEnd    int       `json:"wpmEnd"`
	Completed bool      `json:"completed"`
	Paras     []ParaRun `json:"paras"`
}

// ParaRun is the timing of one paragraph in a run.
type ParaRun struct {
	Planned    float64 `json:"planned"`
	Actual     float64 `json:"actual"`
	Skipped    bool    `json:"skipped,omitempty"`
	NotReached bool    `json:"notReached,omitempty"`
}

const maxRuns = 20

// Open creates the directories if needed.
func Open(dir string) (*Store, error) {
	for _, d := range []string{dir, filepath.Join(dir, "scripts"), filepath.Join(dir, "rehearsals")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	return &Store{Dir: dir}, nil
}

// DefaultDir is %APPDATA%\TeleCyborg on Windows (the OS config dir elsewhere).
func DefaultDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, "TeleCyborg")
}

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,79}$`)

func validID(id string) bool { return idRe.MatchString(id) }

func (s *Store) scriptPath(id string) string { return filepath.Join(s.Dir, "scripts", id+".md") }
func (s *Store) runsPath(id string) string   { return filepath.Join(s.Dir, "rehearsals", id+".json") }

// Slug turns a title into a file-safe id.
func Slug(title string) string {
	t := norm.NFD.String(strings.ToLower(title))
	var b strings.Builder
	dash := false
	for _, r := range t {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 60 {
		out = strings.Trim(out[:60], "-")
	}
	if out == "" {
		out = "script"
	}
	return out
}

func (s *Store) uniqueID(title string) string {
	base := Slug(title)
	id := base
	for i := 2; ; i++ {
		if _, err := os.Stat(s.scriptPath(id)); errors.Is(err, os.ErrNotExist) {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, i)
	}
}

func (s *Store) summarize(id, src string, mod time.Time) (Summary, script.Parsed) {
	p := script.Parse(src)
	wpm := p.Meta.WPM
	return Summary{ID: id, Title: p.Meta.Title, Words: p.Words, Paragraphs: p.Paragraphs, WPM: wpm,
		Target: p.Meta.Target, Duration: p.Duration(wpm), Modified: mod.UTC().Format(time.RFC3339),
		Runs: s.runCount(id)}, p
}

// List returns all scripts, most recently edited first.
func (s *Store) List() ([]Summary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ents, err := os.ReadDir(filepath.Join(s.Dir, "scripts"))
	if err != nil {
		return nil, err
	}
	out := []Summary{}
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		id := strings.TrimSuffix(name, ".md")
		if !validID(id) {
			continue
		}
		b, err := os.ReadFile(s.scriptPath(id))
		if err != nil {
			continue
		}
		info, _ := e.Info()
		sum, _ := s.summarize(id, string(b), info.ModTime())
		out = append(out, sum)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified > out[j].Modified })
	return out, nil
}

// Get loads one script.
func (s *Store) Get(id string) (Script, error) {
	if !validID(id) {
		return Script{}, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.scriptPath(id))
	if err != nil {
		return Script{}, ErrNotFound
	}
	info, _ := os.Stat(s.scriptPath(id))
	sum, p := s.summarize(id, string(b), info.ModTime())
	_, body := script.SplitFrontMatter(string(b))
	return Script{Summary: sum, Source: string(b), Body: body, Parsed: p}, nil
}

// Input is what the editor sends.
type Input struct {
	Title    string  `json:"title"`
	Body     string  `json:"body"`
	WPM      int     `json:"wpm"`
	Target   float64 `json:"target"`
	FontSize int     `json:"fontSize"`
}

func compose(in Input, keep script.Meta) string {
	m := keep
	m.Title = strings.TrimSpace(in.Title)
	if in.WPM > 0 {
		m.WPM = in.WPM
	}
	m.Target = in.Target
	if in.FontSize > 0 {
		m.FontSize = in.FontSize
	}
	if m.Title == "" {
		m.Title = script.Parse(in.Body).Meta.Title
	}
	return script.Compose(m, in.Body)
}

// Create saves a new script and returns its id.
func (s *Store) Create(in Input) (string, error) {
	// a body that brings its own front matter (imported file) keeps its metadata
	keep := script.Parse(in.Body).Meta
	if strings.TrimSpace(in.Title) == "" {
		in.Title = keep.Title
	}
	if in.WPM == 0 {
		in.WPM = keep.WPM
	}
	if in.Target == 0 {
		in.Target = keep.Target
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	src := compose(in, keep)
	id := s.uniqueID(script.Parse(src).Meta.Title)
	return id, writeAtomic(s.scriptPath(id), []byte(src))
}

// Save overwrites a script.
func (s *Store) Save(id string, in Input) error {
	if !validID(id) {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old, err := os.ReadFile(s.scriptPath(id))
	if err != nil {
		return ErrNotFound
	}
	keep := script.Parse(string(old)).Meta
	return writeAtomic(s.scriptPath(id), []byte(compose(in, keep)))
}

// Delete removes a script and its history.
func (s *Store) Delete(id string) error {
	if !validID(id) {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.scriptPath(id)); err != nil {
		return ErrNotFound
	}
	os.Remove(s.runsPath(id))
	return nil
}

func (s *Store) runCount(id string) int {
	b, err := os.ReadFile(s.runsPath(id))
	if err != nil {
		return 0
	}
	var runs []Rehearsal
	if json.Unmarshal(b, &runs) != nil {
		return 0
	}
	return len(runs)
}

// Runs returns the rehearsal history of a script, newest first.
func (s *Store) Runs(id string) ([]Rehearsal, error) {
	if !validID(id) {
		return nil, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	runs := []Rehearsal{}
	if b, err := os.ReadFile(s.runsPath(id)); err == nil {
		_ = json.Unmarshal(b, &runs)
	}
	return runs, nil
}

// AddRun stores a rehearsal (keeps the last 20).
func (s *Store) AddRun(id string, r Rehearsal) error {
	if !validID(id) {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(s.scriptPath(id)); err != nil {
		return ErrNotFound
	}
	runs := []Rehearsal{}
	if b, err := os.ReadFile(s.runsPath(id)); err == nil {
		_ = json.Unmarshal(b, &runs)
	}
	if r.At == "" {
		r.At = time.Now().UTC().Format(time.RFC3339)
	}
	runs = append([]Rehearsal{r}, runs...)
	if len(runs) > maxRuns {
		runs = runs[:maxRuns]
	}
	b, _ := json.MarshalIndent(runs, "", " ")
	return writeAtomic(s.runsPath(id), b)
}

// Settings loads preferences (defaults when missing).
func (s *Store) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := DefaultSettings()
	if b, err := os.ReadFile(filepath.Join(s.Dir, "settings.json")); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	return sanitize(st)
}

// SaveSettings stores preferences.
func (s *Store) SaveSettings(st Settings) (Settings, error) {
	st = sanitize(st)
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.MarshalIndent(st, "", " ")
	return st, writeAtomic(filepath.Join(s.Dir, "settings.json"), b)
}

func sanitize(st Settings) Settings {
	d := DefaultSettings()
	st.WPM = script.ClampWPM(st.WPM)
	if st.FontSize < 20 || st.FontSize > 120 {
		st.FontSize = d.FontSize
	}
	if st.FocusLine < 0.2 || st.FocusLine > 0.6 {
		st.FocusLine = d.FocusLine
	}
	if st.Countdown < 0 || st.Countdown > 10 {
		st.Countdown = d.Countdown
	}
	if st.Motion != "reduced" {
		st.Motion = "full"
	}
	return st
}

func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
