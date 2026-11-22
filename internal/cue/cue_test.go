package cue

import (
	"testing"
	"time"

	"github.com/jose00735/telecyborg/internal/script"
)

const src = `# Opening
keywords: introduce yourself, about you
Hi.

# Netskope results
keywords: netskope, utilisation/adoption
I raised adoption.

=== QA ===
# Q: What is your biggest weakness?
keywords: weakness
I over-prepare.

# Q: What are your salary expectations?
keywords: salary expectations
Market rate.

# Q: Tell me about a conflict with a customer
keywords: conflict, difficult customer
Story.
`

func TestSuggestions(t *testing.T) {
	m := New(script.Parse(src))
	t0 := time.Now()
	got := m.Hear("So tell me, what would you say is your biggest weakness?", t0)
	if len(got) == 0 || got[0].Title != "Q: What is your biggest weakness?" || !got[0].QA {
		t.Fatalf("weakness: %+v", got)
	}
	// Spanish synonym through the built-in group, accents folded
	got = m.Hear("¿Y cuáles son tus debilidades?", t0.Add(30*time.Second))
	if len(got) == 0 || got[0].Sec != 2 {
		t.Fatalf("debilidades: %+v", got)
	}
	// alternative with a slash and stemming: "adoption" vs "adopted" is not close, "adoption rates" is
	got = m.Hear("How did the Netskope adoption rates change?", t0.Add(60*time.Second))
	if len(got) == 0 || got[0].Sec != 1 {
		t.Fatalf("netskope: %+v", got)
	}
	// multi-word with one word between
	got = m.Hear("what are your salary and expectations", t0.Add(120*time.Second))
	if len(got) == 0 || got[0].Sec != 3 {
		t.Fatalf("salary: %+v", got)
	}
	// unrelated talk suggests nothing
	if got = m.Hear("Great, the weather in Bogotá is nice today.", t0.Add(200*time.Second)); len(got) != 0 {
		t.Fatalf("noise: %+v", got)
	}
}

func TestNoRepeatWithoutNewEvidence(t *testing.T) {
	m := New(script.Parse(src))
	t0 := time.Now()
	if got := m.Hear("biggest weakness", t0); len(got) == 0 {
		t.Fatal("expected a suggestion")
	}
	if got := m.Hear("okay and then", t0.Add(3*time.Second)); len(got) != 0 {
		t.Fatalf("repeated without new words: %+v", got)
	}
}
