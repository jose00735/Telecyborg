package speech

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWAV(t *testing.T) {
	w := WAV(make([]byte, 320), 16000)
	if len(w) != 44+320 || string(w[:4]) != "RIFF" || string(w[8:16]) != "WAVEfmt " {
		t.Fatal("bad header")
	}
	if binary.LittleEndian.Uint32(w[24:]) != 16000 || binary.LittleEndian.Uint32(w[40:]) != 320 {
		t.Fatal("bad rate/size")
	}
}

func TestExtractRuntime(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "w.zip")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range append(RuntimeFiles, "SDL2.dll", "whisper-cli.exe") {
		f, _ := zw.Create("Release/" + n)
		f.Write([]byte(n))
	}
	zw.Close()
	os.WriteFile(zp, buf.Bytes(), 0o644)
	out := filepath.Join(dir, "rt")
	if err := ExtractRuntime(zp, out); err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(out)
	if len(ents) != len(RuntimeFiles) {
		t.Fatalf("extracted %d files", len(ents))
	}
}

func TestImportModel(t *testing.T) {
	m := NewManager(t.TempDir(), "")
	if _, err := m.ImportModel(strings.NewReader("lmgg-model-bytes"), "ggml-base-q5_1.bin"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ImportModel(strings.NewReader("garbage!"), "ggml-tiny-q5_1.bin"); err == nil {
		t.Fatal("garbage accepted")
	}
	if _, err := m.ImportModel(strings.NewReader("lmgg"), "evil.exe"); err == nil {
		t.Fatal("bad name accepted")
	}
	st := m.Status()
	if st.Model != "base-q5_1" || !st.Models[0].Installed {
		t.Fatalf("status = %+v", st)
	}
}

func TestClean(t *testing.T) {
	if Clean("  Thank you. ") != "" || Clean(" [BLANK_AUDIO] ") != "" || Clean(" hello   there ") != "hello there" {
		t.Fatal("clean")
	}
}
