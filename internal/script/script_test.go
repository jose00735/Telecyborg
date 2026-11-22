package script

import (
	"math"
	"strings"
	"testing"
)

const sample = `---
title: Terrific pitch
wpm: 150
target: 1:30
custom: kept
---

# Opening
Hi, I'm **Jose**. I work in _customer success_.
Second line of the same paragraph.

[pause 2s]

> smile
<!-- speaker note, hidden -->
At Netskope I managed 2,000+ accounts — really.

[pause]

## Close
keywords: thanks, closing
Thank you.
`

func TestParse(t *testing.T) {
	p := Parse(sample)
	if p.Meta.Title != "Terrific pitch" || p.Meta.WPM != 150 || p.Meta.Target != 90 {
		t.Fatalf("meta: %+v", p.Meta)
	}
	if p.Meta.Extra["custom"] != "kept" {
		t.Fatalf("extra key lost: %+v", p.Meta.Extra)
	}
	if p.Paragraphs != 3 || p.Sections != 2 {
		t.Fatalf("paragraphs=%d sections=%d", p.Paragraphs, p.Sections)
	}
	var paras []Block
	kinds := []string{}
	for _, b := range p.Blocks {
		kinds = append(kinds, b.Kind)
		if b.Kind == "para" {
			paras = append(paras, b)
		}
	}
	if got := strings.Join(kinds, ","); got != "section,para,cue,para,section,para" {
		t.Fatalf("kinds = %s", got)
	}
	// "Hi, I'm Jose. I work in customer success. Second line of the same paragraph." = 14 words
	if paras[0].Words != 14 {
		t.Fatalf("words p0 = %d (%q)", paras[0].Words, paras[0].Text)
	}
	if !strings.Contains(paras[0].HTML, "<strong>Jose</strong>") || !strings.Contains(paras[0].HTML, "<em>customer success</em>") || !strings.Contains(paras[0].HTML, "<br>") {
		t.Fatalf("html p0 = %s", paras[0].HTML)
	}
	if paras[0].Pause != 2 || paras[1].Pause != DefaultPause {
		t.Fatalf("pauses: %v %v", paras[0].Pause, paras[1].Pause)
	}
	// "—" has no letter: not a word. "At Netskope I managed 2,000+ accounts really." = 7
	if paras[1].Words != 7 {
		t.Fatalf("words p1 = %d", paras[1].Words)
	}
	if strings.Contains(paras[1].Text, "speaker note") {
		t.Fatal("comment not removed")
	}
	if paras[2].Section != "Close" || paras[2].Text != "Thank you." {
		t.Fatalf("p2 = %+v", paras[2])
	}
	if p.Pauses != 3.5 {
		t.Fatalf("total pauses = %v", p.Pauses)
	}
}

func TestEscaping(t *testing.T) {
	p := Parse("<script>alert(1)</script> & co")
	if strings.Contains(p.Blocks[0].HTML, "<script>") || !strings.Contains(p.Blocks[0].HTML, "&lt;script&gt;") {
		t.Fatalf("not escaped: %s", p.Blocks[0].HTML)
	}
}

func TestTiming(t *testing.T) {
	p := Parse("one two three four five six\n\n[pause 4]\n\nseven eight nine ten")
	if p.Words != 10 || p.Pauses != 4 {
		t.Fatalf("words=%d pauses=%v", p.Words, p.Pauses)
	}
	if d := p.Duration(60); math.Abs(d-14) > 1e-9 {
		t.Fatalf("duration = %v", d)
	}
	if w := p.FitWPM(9); w != 120 { // 10 words in 5 s of reading = 120 wpm
		t.Fatalf("fit = %d", w)
	}
	if w := p.FitWPM(3); w != MaxWPM {
		t.Fatalf("impossible fit = %d", w)
	}
}

func TestDurations(t *testing.T) {
	cases := map[string]float64{"1:30": 90, "90": 90, "90s": 90, "1:00:05": 3605, "": 0, "x": 0}
	for in, want := range cases {
		if got := ParseDuration(in); got != want {
			t.Errorf("ParseDuration(%q) = %v, want %v", in, got, want)
		}
	}
	if FormatDuration(90) != "1:30" || FormatDuration(5) != "0:05" {
		t.Fatal("FormatDuration")
	}
}

func TestComposeRoundTrip(t *testing.T) {
	src := Compose(Meta{Title: "A", WPM: 999, Target: 75}, "---\ntitle: old\n---\nHello world\n")
	p := Parse(src)
	if p.Meta.Title != "A" || p.Meta.WPM != MaxWPM || p.Meta.Target != 75 || p.Words != 2 {
		t.Fatalf("round trip: %+v words=%d\n%s", p.Meta, p.Words, src)
	}
}

func TestGuessTitle(t *testing.T) {
	if p := Parse("just some words here for a title that is long"); p.Meta.Title != "just some words here for a…" {
		t.Fatalf("title = %q", p.Meta.Title)
	}
}

const packed = `---
title: Interview pitch
target: 1:00
---
Intro line before any heading.

# Opening
keywords: introduce yourself, about you; background
Hi, I'm Jose. I verified this [check].

# Netskope
keywords: netskope, SASE
I ran a book of SaaS accounts.

=== QA ===
# Q: Why do you want this job?
keywords: why this role, motivation
Because it fits.

[pause 2]

# Q: Biggest weakness?
keywords: weakness
I over-prepare.
`

func TestOutlineAndQA(t *testing.T) {
	p := Parse(packed)
	if len(p.Outline) != 4 || p.QAItems != 2 {
		t.Fatalf("outline=%d qa=%d", len(p.Outline), p.QAItems)
	}
	if got := strings.Join(p.Outline[0].Keywords, "|"); got != "introduce yourself|about you|background" {
		t.Fatalf("keywords = %q", got)
	}
	if p.Outline[1].QA || !p.Outline[2].QA || !p.Outline[3].QA {
		t.Fatalf("qa flags: %+v", p.Outline)
	}
	// main flow: 5 + 6 + 7 = 18 words; [check] is not a word
	if p.Paragraphs != 3 || p.Words != 18 {
		t.Fatalf("main paras=%d words=%d", p.Paragraphs, p.Words)
	}
	if p.QAParas != 2 || p.QAWords != 5 {
		t.Fatalf("qa paras=%d words=%d", p.QAParas, p.QAWords)
	}
	if p.Pauses != 0 {
		t.Fatalf("Q&A pauses must not count toward the pitch: %v", p.Pauses)
	}
	if p.Checks != 1 {
		t.Fatalf("checks = %d", p.Checks)
	}
	if p.Outline[2].FirstPara != 0 || p.Outline[3].FirstPara != 1 || p.Outline[1].FirstPara != 2 {
		t.Fatalf("first paras: %+v", p.Outline)
	}
	var check Block
	for _, b := range p.Blocks {
		if b.Kind == "para" && b.Sec == 0 {
			check = b
		}
		if b.Kind == "para" && b.Text == "Intro line before any heading." && b.Sec != -1 {
			t.Fatalf("intro sec = %d", b.Sec)
		}
	}
	if !strings.Contains(check.HTML, `<mark class="check">`) || strings.Contains(check.Text, "[check]") {
		t.Fatalf("check block = %+v", check)
	}
	if p.Meta.Title != "Interview pitch" {
		t.Fatal(p.Meta.Title)
	}
}

func TestQAHeadingWithoutMarker(t *testing.T) {
	p := Parse("# Pitch\nHello.\n\n# Q: Salary expectations?\nkeywords: salary\nLet's talk.\n\n# Close\nBye.")
	if p.QAItems != 1 || p.Paragraphs != 2 || p.Outline[2].QA {
		t.Fatalf("%+v", p.Outline)
	}
}
