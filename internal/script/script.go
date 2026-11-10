// Package script parses TeleCyborg's Markdown script format (docs/requirements/04-script-format.md).
package script

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Meta is the front matter of a script.
type Meta struct {
	Title    string            `json:"title"`
	WPM      int               `json:"wpm"`
	Target   float64           `json:"target"` // seconds, 0 = none
	FontSize int               `json:"fontSize"`
	Lang     string            `json:"lang"`
	Extra    map[string]string `json:"extra,omitempty"`
}

// Block is one rendered unit of the prompter: a section heading, a speaker cue or a paragraph.
type Block struct {
	Kind    string  `json:"kind"` // "section", "cue", "para"
	HTML    string  `json:"html"`
	Text    string  `json:"text"`
	Words   int     `json:"words"`
	Pause   float64 `json:"pause"`             // seconds after this paragraph
	Index   int     `json:"index"`             // paragraph index (paras only), -1 otherwise
	Section string  `json:"section,omitempty"` // current section title
}

// Parsed is a fully parsed script.
type Parsed struct {
	Meta       Meta    `json:"meta"`
	Blocks     []Block `json:"blocks"`
	Words      int     `json:"words"`
	Paragraphs int     `json:"paragraphs"`
	Sections   int     `json:"sections"`
	Pauses     float64 `json:"pauses"`
}

const (
	DefaultWPM   = 140
	MinWPM       = 60
	MaxWPM       = 300
	DefaultPause = 1.5
)

var (
	pauseRe   = regexp.MustCompile(`(?i)^\[pause(?:\s+([0-9]+(?:\.[0-9]+)?)\s*s?)?\]$`)
	headingRe = regexp.MustCompile(`^(#{1,3})\s+(.+?)\s*#*$`)
	commentRe = regexp.MustCompile(`(?s)<!--.*?-->`)
	boldRe    = regexp.MustCompile(`\*\*(.+?)\*\*`)
	asideRe   = regexp.MustCompile(`(^|[\s(])_(.+?)_([\s.,;:!?)]|$)`)
	wordRe    = regexp.MustCompile(`\S+`)
)

// SplitFrontMatter separates "---" front matter from the body.
func SplitFrontMatter(src string) (map[string]string, string) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.TrimPrefix(src, "\ufeff")
	fm := map[string]string{}
	if !strings.HasPrefix(src, "---\n") {
		return fm, src
	}
	rest := src[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return fm, src
	}
	for _, line := range strings.Split(rest[:end], "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fm[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	body := rest[end+4:]
	if i := strings.IndexByte(body, '\n'); i >= 0 {
		body = body[i+1:]
	} else {
		body = ""
	}
	return fm, body
}

// ParseDuration accepts "m:ss", "h:mm:ss", "90", "90s" and returns seconds (0 if empty/invalid).
func ParseDuration(s string) float64 {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "s"))
	if s == "" {
		return 0
	}
	parts := strings.Split(s, ":")
	total := 0.0
	for _, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil || v < 0 {
			return 0
		}
		total = total*60 + v
	}
	return total
}

// FormatDuration renders seconds as m:ss.
func FormatDuration(sec float64) string {
	if sec <= 0 {
		return ""
	}
	t := int(sec + 0.5)
	return strconv.Itoa(t/60) + ":" + twoDigits(t%60)
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// CountWords counts whitespace-separated tokens that contain a letter or digit.
func CountWords(s string) int {
	n := 0
	for _, w := range wordRe.FindAllString(s, -1) {
		for _, r := range w {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				n++
				break
			}
		}
	}
	return n
}

func metaFrom(fm map[string]string) Meta {
	m := Meta{WPM: DefaultWPM, Extra: map[string]string{}}
	for k, v := range fm {
		switch strings.ToLower(k) {
		case "title":
			m.Title = v
		case "wpm":
			if n, err := strconv.Atoi(v); err == nil {
				m.WPM = ClampWPM(n)
			}
		case "target":
			m.Target = ParseDuration(v)
		case "fontsize":
			if n, err := strconv.Atoi(v); err == nil && n >= 16 && n <= 120 {
				m.FontSize = n
			}
		case "lang":
			m.Lang = v
		default:
			m.Extra[k] = v
		}
	}
	if len(m.Extra) == 0 {
		m.Extra = nil
	}
	return m
}

// ClampWPM keeps a pace inside the supported range.
func ClampWPM(n int) int {
	if n < MinWPM {
		return MinWPM
	}
	if n > MaxWPM {
		return MaxWPM
	}
	return n
}

// inline renders the small Markdown subset (bold, aside) on escaped text.
func inline(s string) string {
	s = html.EscapeString(s)
	s = boldRe.ReplaceAllString(s, "<strong>$1</strong>")
	s = asideRe.ReplaceAllString(s, "$1<em>$2</em>$3")
	return s
}

// plain strips the inline markers for word counting and search.
func plain(s string) string {
	s = boldRe.ReplaceAllString(s, "$1")
	s = asideRe.ReplaceAllString(s, "$1$2$3")
	return s
}

// Parse parses a full script file (front matter + body).
func Parse(src string) Parsed {
	fm, body := SplitFrontMatter(src)
	p := Parsed{Meta: metaFrom(fm)}
	body = commentRe.ReplaceAllString(body, "")

	section := ""
	var para []string
	flush := func() {
		if len(para) == 0 {
			return
		}
		lines := make([]string, len(para))
		plains := make([]string, len(para))
		for i, l := range para {
			lines[i] = inline(l)
			plains[i] = plain(l)
		}
		text := strings.Join(plains, "\n")
		b := Block{Kind: "para", HTML: strings.Join(lines, "<br>"), Text: text, Words: CountWords(text),
			Index: p.Paragraphs, Section: section}
		p.Blocks = append(p.Blocks, b)
		p.Words += b.Words
		p.Paragraphs++
		para = nil
	}
	lastPara := func() *Block {
		for i := len(p.Blocks) - 1; i >= 0; i-- {
			if p.Blocks[i].Kind == "para" {
				return &p.Blocks[i]
			}
		}
		return nil
	}

	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "":
			flush()
		case pauseRe.MatchString(line):
			flush()
			d := DefaultPause
			if m := pauseRe.FindStringSubmatch(line); m[1] != "" {
				d, _ = strconv.ParseFloat(m[1], 64)
			}
			if lp := lastPara(); lp != nil {
				lp.Pause += d
				p.Pauses += d
			}
		case headingRe.MatchString(line):
			flush()
			m := headingRe.FindStringSubmatch(line)
			section = strings.TrimSpace(m[2])
			p.Blocks = append(p.Blocks, Block{Kind: "section", HTML: inline(section), Text: section, Index: -1, Section: section})
			p.Sections++
		case strings.HasPrefix(line, ">"):
			flush()
			cue := strings.TrimSpace(strings.TrimLeft(line, ">"))
			if cue != "" {
				p.Blocks = append(p.Blocks, Block{Kind: "cue", HTML: inline(cue), Text: cue, Index: -1, Section: section})
			}
		case strings.HasPrefix(strings.ToLower(line), "keywords:"):
			// stage 2 metadata; not shown in stage 1
			flush()
		default:
			para = append(para, line)
		}
	}
	flush()
	if p.Meta.Title == "" {
		p.Meta.Title = guessTitle(p)
	}
	return p
}

func guessTitle(p Parsed) string {
	for _, b := range p.Blocks {
		if b.Kind == "section" {
			return b.Text
		}
	}
	for _, b := range p.Blocks {
		if b.Kind == "para" {
			words := strings.Fields(b.Text)
			if len(words) > 6 {
				words = words[:6]
			}
			return strings.Join(words, " ") + "…"
		}
	}
	return "Untitled script"
}

// Duration is the planned length at the given pace (reading time plus pauses).
func (p Parsed) Duration(wpm int) float64 {
	if wpm <= 0 {
		wpm = DefaultWPM
	}
	return float64(p.Words)*60/float64(wpm) + p.Pauses
}

// FitWPM returns the pace that makes the script last target seconds (clamped).
func (p Parsed) FitWPM(target float64) int {
	avail := target - p.Pauses
	if avail <= 0 || p.Words == 0 {
		return MaxWPM
	}
	return ClampWPM(int(float64(p.Words)*60/avail + 0.5))
}

// Compose builds a script file from metadata and a body (front matter is regenerated).
func Compose(m Meta, body string) string {
	_, body = SplitFrontMatter(body)
	var b strings.Builder
	b.WriteString("---\n")
	if m.Title != "" {
		b.WriteString("title: " + strings.ReplaceAll(m.Title, "\n", " ") + "\n")
	}
	if m.WPM > 0 {
		b.WriteString("wpm: " + strconv.Itoa(ClampWPM(m.WPM)) + "\n")
	}
	if m.Target > 0 {
		b.WriteString("target: " + FormatDuration(m.Target) + "\n")
	}
	if m.FontSize > 0 {
		b.WriteString("fontSize: " + strconv.Itoa(m.FontSize) + "\n")
	}
	if m.Lang != "" {
		b.WriteString("lang: " + m.Lang + "\n")
	}
	for k, v := range m.Extra {
		b.WriteString(k + ": " + v + "\n")
	}
	b.WriteString("---\n")
	b.WriteString(strings.TrimLeft(body, "\n"))
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}
