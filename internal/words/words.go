// Package words normalises spoken and written text so transcripts can be compared with scripts
// in English, Spanish and Portuguese: lower case, no accents, no punctuation, light stemming.
package words

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Fold lower-cases s and removes accents and punctuation, keeping letters, digits and spaces.
func Fold(s string) string {
	t := norm.NFD.String(strings.ToLower(s))
	var b strings.Builder
	b.Grow(len(t))
	space := true
	for _, r := range t {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			space = false
		case r == '\'' || r == '’':
			// "I'm" -> "im", "don't" -> "dont"
			continue
		default:
			if !space {
				b.WriteByte(' ')
				space = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// Tokens returns the folded words of s.
func Tokens(s string) []string { return strings.Fields(Fold(s)) }

// Stem strips common English, Spanish and Portuguese suffixes so "pricing", "prices" and
// "precios" style variants meet. It is deliberately light: matching, not linguistics.
func Stem(w string) string {
	if len(w) <= 3 {
		return w
	}
	for _, suf := range suffixes {
		if strings.HasSuffix(w, suf) && len(w)-len(suf) >= 3 {
			return w[:len(w)-len(suf)]
		}
	}
	return w
}

// longest first
var suffixes = []string{
	"amientos", "imientos", "aciones", "iciones", "amento", "amiento", "imiento", "mente",
	"acion", "icion", "acoes", "ation", "ments", "ness", "ings", "ment", "cion", "coes", "cao",
	"ing", "ies", "ers", "es", "ed", "er", "ly", "s", "a", "o", "e",
}

// StemAll stems a token list.
func StemAll(ts []string) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = Stem(t)
	}
	return out
}

// Stop words that carry no meaning for cue matching (EN/ES/PT).
var stop = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`a an the and or but of to in on at for with by from as is are was were be been
	it its this that these those i you he she we they me my your our their his her them us do does did have has had
	so if then than there here what which who whom how about can could would should will just also very really
	el la los las un una unos unas y o pero de del al en con por para como es son era fue ser que se lo le les
	mi tu su sus nuestro yo usted ustedes nos muy mas ya esto eso este esta estos estas aqui
	o os as um uma e mas do da dos das no na nos nas em com por para como e sao era foi ser que se eu voce meu sua
	um uh eh hmm mm ah oh like okay ok yeah`) {
		stop[w] = true
	}
}

// IsStop reports whether a folded token is a stop word.
func IsStop(w string) bool { return stop[w] }

// Close reports whether two folded tokens are the same word allowing for small transcription
// errors: equal, equal stems, or one edit apart for words of five letters or more.
func Close(a, b string) bool {
	if a == b {
		return true
	}
	if len(a) >= 4 && len(b) >= 4 && Stem(a) == Stem(b) {
		return true
	}
	if len(a) >= 5 && len(b) >= 5 && abs(len(a)-len(b)) <= 1 {
		return editDistanceAtMost1(a, b)
	}
	return false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func editDistanceAtMost1(a, b string) bool {
	ra, rb := []rune(a), []rune(b)
	if len(ra) < len(rb) {
		ra, rb = rb, ra
	}
	i, j, edits := 0, 0, 0
	for i < len(ra) && j < len(rb) {
		if ra[i] == rb[j] {
			i++
			j++
			continue
		}
		edits++
		if edits > 1 {
			return false
		}
		if len(ra) == len(rb) {
			i++
			j++
		} else {
			i++
		}
	}
	edits += (len(ra) - i) + (len(rb) - j)
	return edits <= 1
}

// fillerRe finds hesitations ("um", "eh"), filler phrases ("o sea", "you know") and words that
// are fillers only when set off by a comma ("like,", "este,", "pues,", "tipo,", "né,").
var fillerRe = regexp.MustCompile(`(?i)(?:^|[^\p{L}])(um+|uh+m*|erm+|eh+|mm+|hmm+|ah+m*|o sea|you know|i mean|(?:like|este|pues|tipo|né|bueno),)`)

// CountFillers counts filler words in a transcript.
func CountFillers(text string) map[string]int {
	out := map[string]int{}
	t := strings.ToLower(text)
	for {
		loc := fillerRe.FindStringSubmatchIndex(t)
		if loc == nil {
			break
		}
		w := strings.TrimSuffix(t[loc[2]:loc[3]], ",")
		switch {
		case strings.HasPrefix(w, "um"), strings.HasPrefix(w, "uh"):
			w = "um"
		case strings.HasPrefix(w, "eh"), strings.HasPrefix(w, "ah"):
			w = "eh"
		case strings.HasPrefix(w, "mm"), strings.HasPrefix(w, "hm"):
			w = "hmm"
		case strings.HasPrefix(w, "er"):
			w = "erm"
		}
		out[w]++
		t = t[loc[3]:]
	}
	return out
}
