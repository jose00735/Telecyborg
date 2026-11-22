// Package pack keeps content packs: the brief for one occasion (interview, customer meeting,
// demo), the prompt TeleCyborg writes for Claude, and the pitch + Q&A that comes back
// (docs/requirements/08-content-packs.md).
package pack

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jose00735/telecyborg/internal/script"
)

// ErrNotFound is returned for unknown packs.
var ErrNotFound = errors.New("pack not found")

// Pack is the brief for one occasion.
type Pack struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Occasion  string   `json:"occasion"` // interview, customer meeting, demo, other
	Audience  string   `json:"audience"`
	Goal      string   `json:"goal"`
	Target    string   `json:"target"` // m:ss for the pitch
	WPM       int      `json:"wpm"`
	Lang      string   `json:"lang"` // en, es, pt
	Topics    []string `json:"topics"`
	Facts     []string `json:"facts"`
	Avoid     []string `json:"avoid"`
	Sources   string   `json:"sources"`
	QACount   int      `json:"qaCount"`
	AnswerMax int      `json:"answerMax"` // seconds per answer
	ScriptID  string   `json:"scriptId,omitempty"`
	Created   string   `json:"created"`
	Modified  string   `json:"modified"`
}

// Store is the packs folder.
type Store struct {
	Dir string
	mu  sync.Mutex
}

// Open creates the packs folder.
func Open(dir string) (*Store, error) {
	d := filepath.Join(dir, "packs")
	return &Store{Dir: d}, os.MkdirAll(d, 0o755)
}

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,79}$`)

func (s *Store) path(id, name string) string { return filepath.Join(s.Dir, id, name) }

// Normalize fills defaults and trims lists.
func Normalize(p Pack) Pack {
	p.Title = strings.TrimSpace(p.Title)
	if p.Title == "" {
		p.Title = "Untitled pack"
	}
	switch p.Occasion {
	case "interview", "customer meeting", "demo", "other":
	default:
		p.Occasion = "interview"
	}
	switch p.Lang {
	case "en", "es", "pt":
	default:
		p.Lang = "en"
	}
	if p.WPM == 0 {
		p.WPM = script.DefaultWPM
	}
	p.WPM = script.ClampWPM(p.WPM)
	if script.ParseDuration(p.Target) <= 0 {
		p.Target = "2:00"
	}
	p.Target = script.FormatDuration(script.ParseDuration(p.Target))
	if p.QACount <= 0 || p.QACount > 30 {
		p.QACount = 8
	}
	if p.AnswerMax < 20 || p.AnswerMax > 180 {
		p.AnswerMax = 60
	}
	p.Topics, p.Facts, p.Avoid = clean(p.Topics), clean(p.Facts), clean(p.Avoid)
	return p
}

func clean(in []string) []string {
	out := []string{}
	for _, l := range in {
		for _, x := range strings.Split(l, "\n") {
			x = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(x), "-*• "))
			if x != "" {
				out = append(out, x)
			}
		}
	}
	return out
}

// List returns all packs, newest first.
func (s *Store) List() ([]Pack, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ents, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, err
	}
	out := []Pack{}
	for _, e := range ents {
		if !e.IsDir() || !idRe.MatchString(e.Name()) {
			continue
		}
		if p, err := s.read(e.Name()); err == nil {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified > out[j].Modified })
	return out, nil
}

func (s *Store) read(id string) (Pack, error) {
	b, err := os.ReadFile(s.path(id, "pack.json"))
	if err != nil {
		return Pack{}, ErrNotFound
	}
	var p Pack
	if err := json.Unmarshal(b, &p); err != nil {
		return Pack{}, err
	}
	p.ID = id
	return p, nil
}

// Get loads one pack.
func (s *Store) Get(id string) (Pack, error) {
	if !idRe.MatchString(id) {
		return Pack{}, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(id)
}

// Save creates (empty ID) or updates a pack and rewrites pack.md and claude-brief.md.
func (s *Store) Save(p Pack, slug func(string) string) (Pack, error) {
	p = Normalize(p)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	if p.ID == "" {
		base := slug(p.Title)
		id := base
		for i := 2; ; i++ {
			if _, err := os.Stat(filepath.Join(s.Dir, id)); errors.Is(err, os.ErrNotExist) {
				break
			}
			id = fmt.Sprintf("%s-%d", base, i)
		}
		p.ID = id
		p.Created = now
	} else {
		if !idRe.MatchString(p.ID) {
			return Pack{}, ErrNotFound
		}
		old, err := s.read(p.ID)
		if err != nil {
			return Pack{}, ErrNotFound
		}
		p.Created = old.Created
		if p.ScriptID == "" {
			p.ScriptID = old.ScriptID
		}
	}
	p.Modified = now
	if err := os.MkdirAll(filepath.Join(s.Dir, p.ID), 0o755); err != nil {
		return Pack{}, err
	}
	b, _ := json.MarshalIndent(p, "", " ")
	if err := os.WriteFile(s.path(p.ID, "pack.json"), b, 0o644); err != nil {
		return Pack{}, err
	}
	_ = os.WriteFile(s.path(p.ID, "pack.md"), []byte(Markdown(p)), 0o644)
	_ = os.WriteFile(s.path(p.ID, "claude-brief.md"), []byte(Brief(p)), 0o644)
	return p, nil
}

// SetScript links the imported script and keeps Claude's answer as answer.md.
func (s *Store) SetScript(id, scriptID, answer string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.read(id)
	if err != nil {
		return err
	}
	p.ScriptID = scriptID
	p.Modified = time.Now().UTC().Format(time.RFC3339)
	b, _ := json.MarshalIndent(p, "", " ")
	if err := os.WriteFile(s.path(id, "pack.json"), b, 0o644); err != nil {
		return err
	}
	return os.WriteFile(s.path(id, "answer.md"), []byte(answer), 0o644)
}

// Delete removes a pack folder (its script stays in the library).
func (s *Store) Delete(id string) error {
	if !idRe.MatchString(id) {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(filepath.Join(s.Dir, id)); err != nil {
		return ErrNotFound
	}
	return os.RemoveAll(filepath.Join(s.Dir, id))
}

var occasionWords = map[string]string{
	"interview": "a job interview", "customer meeting": "a customer meeting", "demo": "a product demo", "other": "a presentation",
}

var langNames = map[string]string{"en": "English", "es": "Spanish", "pt": "Portuguese"}

// Markdown renders the pack as a readable brief (pack.md).
func Markdown(p Pack) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", p.Title)
	fmt.Fprintf(&b, "- Occasion: %s\n- Audience: %s\n- Goal: %s\n- Pitch length: %s at %d wpm\n- Language: %s\n- Likely questions: %d (answers up to %d s)\n\n",
		p.Occasion, orDash(p.Audience), orDash(p.Goal), p.Target, p.WPM, langNames[p.Lang], p.QACount, p.AnswerMax)
	list(&b, "Topics", p.Topics)
	list(&b, "Must-mention facts", p.Facts)
	list(&b, "Avoid", p.Avoid)
	if strings.TrimSpace(p.Sources) != "" {
		b.WriteString("## Sources\n\n" + strings.TrimSpace(p.Sources) + "\n")
	}
	return b.String()
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return strings.TrimSpace(s)
}

func list(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "## %s\n\n", title)
	for _, it := range items {
		fmt.Fprintf(b, "- %s\n", it)
	}
	b.WriteString("\n")
}

// Brief is the prompt for Claude (claude-brief.md): who, for whom, what for, timing per
// section, the facts and sources to use, and the exact output format TeleCyborg imports.
func Brief(p Pack) string {
	p = Normalize(p)
	target := script.ParseDuration(p.Target)
	totalWords := int(target*float64(p.WPM)/60 + 0.5)
	sections := len(p.Topics) + 2 // opening + topics + close
	per := totalWords / sections
	answerWords := p.AnswerMax * p.WPM / 60
	lang := langNames[p.Lang]

	var b strings.Builder
	fmt.Fprintf(&b, "You are helping me prepare for %s. Write a spoken pitch I will read from a teleprompter, plus the questions I am likely to be asked with short spoken answers.\n\n", occasionWords[p.Occasion])
	b.WriteString("## Context\n\n")
	fmt.Fprintf(&b, "- Title: %s\n- Audience: %s\n- My goal: %s\n- Language: write everything in %s.\n\n", p.Title, orDash(p.Audience), orDash(p.Goal), lang)
	b.WriteString("## Timing\n\n")
	fmt.Fprintf(&b, "- The pitch must last %s at %d words per minute: about %d words in total, not counting headings and keyword lines.\n", p.Target, p.WPM, totalWords)
	fmt.Fprintf(&b, "- Use %d sections: an opening, one section per topic below, and a close. Aim for about %d words per section; shift words toward the topics that matter most to this audience.\n", sections, per)
	fmt.Fprintf(&b, "- Write %d likely questions. Each answer must fit in %d seconds (at most %d words).\n\n", p.QACount, p.AnswerMax, answerWords)
	if len(p.Topics) > 0 {
		b.WriteString("## Topics (in this order)\n\n")
		for i, t := range p.Topics {
			fmt.Fprintf(&b, "%d. %s\n", i+1, t)
		}
		b.WriteString("\n")
	}
	if len(p.Facts) > 0 {
		b.WriteString("## Facts I must mention (use them exactly as written)\n\n")
		for _, f := range p.Facts {
			fmt.Fprintf(&b, "- %s\n", f)
		}
		b.WriteString("\n")
	}
	if len(p.Avoid) > 0 {
		b.WriteString("## Avoid\n\n")
		for _, f := range p.Avoid {
			fmt.Fprintf(&b, "- %s\n", f)
		}
		b.WriteString("\n")
	}
	if strings.TrimSpace(p.Sources) != "" {
		b.WriteString("## Sources (the only material you may draw facts from)\n\n")
		b.WriteString(strings.TrimSpace(p.Sources) + "\n\n")
	}
	b.WriteString(`## Rules for facts

- Use only the facts and sources above. Never invent numbers, names, dates, customers or results.
- If a sentence needs a fact you do not have, write the sentence with a placeholder and put [check] right after it, so I can verify or replace it.
- Write for the ear: short sentences, plain words, first person, natural pauses. No bullet points inside the text.

## Keywords

Under every heading, add one line that starts with ` + "`keywords:`" + ` followed by 3–8 comma-separated words or short phrases the OTHER person would say that should bring that section up (for example "pricing, cost, budget"). Include synonyms and, if the other person may speak another language, the same words in that language. Use a slash for alternatives of one idea: "price/cost".

## Output format

Reply with ONLY the Markdown below inside one code block, nothing before or after it. Keep the exact structure: the front matter, "# " headings, the keywords line under each heading, blank lines between paragraphs, the line "=== QA ===" before the questions, and "# Q: " headings for the questions. You may add "[pause 1]" on its own line where I should pause, "**word**" to stress a word, and "> note" lines as short stage directions for me (they are not read aloud).

` + "```markdown" + `
---
title: ` + p.Title + `
target: ` + p.Target + `
wpm: ` + fmt.Sprint(p.WPM) + `
lang: ` + p.Lang + `
---
# <Section title>
keywords: <comma-separated words the listener would say>
<paragraph>

<paragraph>

=== QA ===
# Q: <likely question>
keywords: <comma-separated words in the question>
<answer paragraphs>
` + "```" + `
`)
	return b.String()
}

// Preview is what an import would create.
type Preview struct {
	Body          string   `json:"body"` // cleaned Markdown that will be saved
	Title         string   `json:"title"`
	Sections      int      `json:"sections"`
	QA            int      `json:"qa"`
	Words         int      `json:"words"`
	Duration      float64  `json:"duration"`
	Target        float64  `json:"target"`
	Checks        int      `json:"checks"`
	Warnings      []string `json:"warnings"`
	Errors        []string `json:"errors"`
	OK            bool     `json:"ok"`
	SectionTitles []string `json:"sectionTitles"`
}

var fenceRe = regexp.MustCompile("(?s)```(?:markdown|md)?[ \t]*\n(.*?)\n```")

// Extract pulls the Markdown out of Claude's reply: the first fenced block when there is one.
func Extract(answer string) string {
	answer = strings.ReplaceAll(answer, "\r\n", "\n")
	if m := fenceRe.FindStringSubmatch(answer); m != nil {
		return strings.TrimSpace(m[1]) + "\n"
	}
	t := strings.TrimSpace(answer)
	if i := strings.Index(t, "---\n"); i > 0 && i < 400 {
		t = t[i:]
	}
	return t + "\n"
}

// Validate checks an answer against the format and the pack.
func Validate(answer string, p Pack) Preview {
	p = Normalize(p)
	body := Extract(answer)
	ps := script.Parse(body)
	pv := Preview{Body: body, Title: ps.Meta.Title, Sections: ps.Sections - ps.QAItems, QA: ps.QAItems, Words: ps.Words,
		Target: ps.Meta.Target, Checks: ps.Checks, Warnings: []string{}, Errors: []string{}, SectionTitles: []string{}}
	wpm := ps.Meta.WPM
	fm, _ := script.SplitFrontMatter(body)
	if _, has := fm["wpm"]; !has {
		wpm = p.WPM
	}
	if _, has := fm["target"]; !has {
		pv.Target = script.ParseDuration(p.Target)
	}
	if _, has := fm["title"]; !has {
		pv.Title = p.Title
	}
	pv.Duration = ps.Duration(wpm)
	for _, o := range ps.Outline {
		pv.SectionTitles = append(pv.SectionTitles, o.Title)
	}
	if ps.Paragraphs == 0 {
		pv.Errors = append(pv.Errors, "No pitch text found. Paste Claude's whole reply, including the code block.")
	}
	if pv.Sections == 0 {
		pv.Errors = append(pv.Errors, "No sections found: headings must start with \"# \".")
	}
	if ps.QAItems == 0 {
		pv.Warnings = append(pv.Warnings, "No questions found (\"=== QA ===\" and \"# Q:\" headings). The pitch will import without cue answers.")
	} else if ps.QAItems < p.QACount/2 {
		pv.Warnings = append(pv.Warnings, fmt.Sprintf("Only %d questions; the pack asked for %d.", ps.QAItems, p.QACount))
	}
	var noKw []string
	for _, o := range ps.Outline {
		if len(o.Keywords) == 0 {
			noKw = append(noKw, o.Title)
		}
		if o.QA && o.Paras == 0 {
			pv.Warnings = append(pv.Warnings, fmt.Sprintf("%q has no answer text.", o.Title))
		}
		if o.QA && float64(o.Words)*60/float64(wpm) > float64(p.AnswerMax)*1.25 {
			pv.Warnings = append(pv.Warnings, fmt.Sprintf("%q runs about %s, longer than %d s.", o.Title, script.FormatDuration(float64(o.Words)*60/float64(wpm)), p.AnswerMax))
		}
	}
	if len(noKw) > 0 {
		pv.Warnings = append(pv.Warnings, fmt.Sprintf("%d section(s) have no keywords line; they are matched by title only: %s.", len(noKw), strings.Join(firstN(noKw, 4), "; ")))
	}
	if pv.Target > 0 && pv.Duration > pv.Target*1.15 {
		pv.Warnings = append(pv.Warnings, fmt.Sprintf("The pitch reads in %s at %d wpm, over the %s target. Fit to time will raise the pace, or cut text.", script.FormatDuration(pv.Duration), wpm, script.FormatDuration(pv.Target)))
	}
	if pv.Checks > 0 {
		pv.Warnings = append(pv.Warnings, fmt.Sprintf("%d [check] mark(s): facts Claude could not confirm. Verify them in the editor before you present.", pv.Checks))
	}
	pv.OK = len(pv.Errors) == 0
	return pv
}

func firstN(s []string, n int) []string {
	if len(s) > n {
		return append(s[:n:n], "…")
	}
	return s
}

// WithPack makes sure the script body's front matter names the pack and carries its defaults.
func WithPack(body string, p Pack) string {
	fm, rest := script.SplitFrontMatter(body)
	if fm["title"] == "" {
		fm["title"] = p.Title
	}
	if fm["target"] == "" {
		fm["target"] = p.Target
	}
	if fm["wpm"] == "" {
		fm["wpm"] = fmt.Sprint(p.WPM)
	}
	if fm["lang"] == "" {
		fm["lang"] = p.Lang
	}
	fm["pack"] = p.ID
	keys := []string{"title", "target", "wpm", "lang", "pack"}
	var extra []string
	for k := range fm {
		if !contains(keys, k) {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	var b strings.Builder
	b.WriteString("---\n")
	for _, k := range append(keys, extra...) {
		fmt.Fprintf(&b, "%s: %s\n", k, strings.ReplaceAll(fm[k], "\n", " "))
	}
	b.WriteString("---\n")
	b.WriteString(strings.TrimLeft(rest, "\n"))
	return b.String()
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// Export zips a pack folder plus its script (as script.md).
func (s *Store) Export(id string, scriptSource string) ([]byte, error) {
	if !idRe.MatchString(id) {
		return nil, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.Dir, id)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, ErrNotFound
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") && !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		w, _ := zw.Create(id + "/" + e.Name())
		_, _ = w.Write(data)
	}
	if scriptSource != "" {
		w, _ := zw.Create(id + "/script.md")
		_, _ = w.Write([]byte(scriptSource))
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Imported is the result of importing a pack zip.
type Imported struct {
	Pack   Pack
	Script string // script.md content, "" when absent
}

// ImportZip reads a zip made by Export and stores it as a new pack (the id may change).
func (s *Store) ImportZip(data []byte, slug func(string) string) (Imported, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Imported{}, errors.New("this is not a TeleCyborg pack (.zip)")
	}
	files := map[string][]byte{}
	for _, f := range zr.File {
		name := filepath.Base(strings.ReplaceAll(f.Name, "\\", "/"))
		switch name {
		case "pack.json", "answer.md", "script.md":
		default:
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(rc, 8<<20))
		rc.Close()
		files[name] = b
	}
	pj, ok := files["pack.json"]
	if !ok {
		return Imported{}, errors.New("the zip has no pack.json")
	}
	var p Pack
	if err := json.Unmarshal(pj, &p); err != nil {
		return Imported{}, errors.New("pack.json is not valid")
	}
	p.ID, p.ScriptID = "", ""
	p, err = s.Save(p, slug)
	if err != nil {
		return Imported{}, err
	}
	if a, ok := files["answer.md"]; ok {
		_ = os.WriteFile(s.path(p.ID, "answer.md"), a, 0o644)
	}
	return Imported{Pack: p, Script: string(files["script.md"])}, nil
}

// ---- phone sync (internal/hub) ----

// IDs lists pack ids.
func (s *Store) IDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ents, _ := os.ReadDir(s.Dir)
	out := []string{}
	for _, e := range ents {
		if e.IsDir() && idRe.MatchString(e.Name()) {
			if _, err := os.Stat(s.path(e.Name(), "pack.json")); err == nil {
				out = append(out, e.Name())
			}
		}
	}
	return out
}

// Raw returns pack.json and answer.md exactly as stored, and the newest modification time.
func (s *Store) Raw(id string) (packJSON, answer []byte, mod time.Time, err error) {
	if !idRe.MatchString(id) {
		return nil, nil, time.Time{}, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	packJSON, err = os.ReadFile(s.path(id, "pack.json"))
	if err != nil {
		return nil, nil, time.Time{}, ErrNotFound
	}
	st, _ := os.Stat(s.path(id, "pack.json"))
	mod = st.ModTime()
	if a, err := os.ReadFile(s.path(id, "answer.md")); err == nil {
		answer = a
		if st, err := os.Stat(s.path(id, "answer.md")); err == nil && st.ModTime().After(mod) {
			mod = st.ModTime()
		}
	}
	return packJSON, answer, mod, nil
}

// PutRaw stores a pack from another device exactly as given and rebuilds pack.md and the brief.
func (s *Store) PutRaw(id string, packJSON, answer []byte) error {
	if !idRe.MatchString(id) {
		return ErrNotFound
	}
	var p Pack
	if err := json.Unmarshal(packJSON, &p); err != nil {
		return fmt.Errorf("pack.json is not valid: %w", err)
	}
	p.ID = id
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Join(s.Dir, id), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(s.path(id, "pack.json"), packJSON, 0o644); err != nil {
		return err
	}
	if len(answer) > 0 {
		if err := os.WriteFile(s.path(id, "answer.md"), answer, 0o644); err != nil {
			return err
		}
	} else {
		os.Remove(s.path(id, "answer.md"))
	}
	_ = os.WriteFile(s.path(id, "pack.md"), []byte(Markdown(Normalize(p))), 0o644)
	_ = os.WriteFile(s.path(id, "claude-brief.md"), []byte(Brief(p)), 0o644)
	return nil
}
