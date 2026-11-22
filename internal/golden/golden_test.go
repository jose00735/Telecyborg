// Package golden writes and checks the fixtures shared with the Android app (android/golden).
package golden

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jose00735/telecyborg/internal/hub"
	"github.com/jose00735/telecyborg/internal/pack"
	"github.com/jose00735/telecyborg/internal/script"
)

var update = flag.Bool("update", false, "rewrite the expected files")

const dir = "../../android/golden"

func check(t *testing.T, name string, got []byte) {
	p := filepath.Join(dir, name)
	if *update {
		if err := os.WriteFile(p, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("%s missing: run go test ./internal/golden -update", name)
	}
	if string(want) != string(got) {
		t.Fatalf("%s changed; if intended run -update and fix the Kotlin side", name)
	}
}

func TestParse(t *testing.T) {
	for _, n := range []string{"pitch", "plain"} {
		src, _ := os.ReadFile(filepath.Join(dir, n+".md"))
		p := script.Parse(string(src))
		b, _ := json.MarshalIndent(map[string]any{"parsed": p, "duration": p.Duration(p.Meta.WPM), "fitWpm90": p.FitWPM(90)}, "", " ")
		check(t, n+".parsed.json", append(b, '\n'))
	}
}

func TestBriefAndValidate(t *testing.T) {
	src, _ := os.ReadFile(filepath.Join(dir, "pack.json"))
	var p pack.Pack
	if err := json.Unmarshal(src, &p); err != nil {
		t.Fatal(err)
	}
	p.ID = "terrific-interview"
	check(t, "pack.brief.txt", []byte(pack.Brief(p)))
	answer, _ := os.ReadFile(filepath.Join(dir, "pitch.md"))
	reply := "Here it is:\n\n```markdown\n" + string(answer) + "```\nGood luck"
	pv := pack.Validate(reply, p)
	pv.Body = strings.TrimSpace(pv.Body)
	b, _ := json.MarshalIndent(map[string]any{"preview": pv, "withPack": pack.WithPack(pack.Extract(reply), p)}, "", " ")
	check(t, "pack.validate.json", append(b, '\n'))
}

func TestHash(t *testing.T) {
	src, _ := os.ReadFile(filepath.Join(dir, "pitch.md"))
	check(t, "pitch.hash.txt", []byte(hub.Hash(string(src))+"\n"))
}
