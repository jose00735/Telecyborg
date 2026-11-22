package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jose00735/telecyborg/internal/pack"
	"github.com/jose00735/telecyborg/internal/store"
)

const claudeAnswer = "Here is your pitch:\n\n```markdown\n---\ntitle: Terrific interview\ntarget: 0:30\nwpm: 140\nlang: en\n---\n# Opening\nkeywords: introduce yourself, about you\nHi, I'm Jose. I help customers adopt security platforms.\n\n# Netskope\nkeywords: netskope, adoption\nAt Netskope I raised adoption from fifty to seventy percent [check].\n\n=== QA ===\n# Q: Why do you want this role?\nkeywords: why this role, motivation\nBecause it joins support depth and customer success.\n\n# Q: What is your biggest weakness?\nkeywords: weakness\nI over-prepare, so I now time-box it.\n```\n\nGood luck!"

func decode(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
}

func TestPackFlow(t *testing.T) {
	s, h := newTest(t)
	tok := s.Token
	rec := do(h, tok, "POST", "/api/packs", pack.Pack{Title: "Terrific interview", Occasion: "interview", Target: "0:30", Lang: "en",
		Topics: []string{"Netskope results\n- Fortinet support"}, Facts: []string{"moved an account from below 50% to 70% utilisation"}, QACount: 2})
	if rec.Code != 201 {
		t.Fatalf("create pack: %d %s", rec.Code, rec.Body)
	}
	var p pack.Pack
	decode(t, rec, &p)
	if p.ID != "terrific-interview" || len(p.Topics) != 2 {
		t.Fatalf("pack = %+v", p)
	}
	rec = do(h, tok, "GET", "/api/packs/"+p.ID+"/brief", nil)
	var br map[string]string
	decode(t, rec, &br)
	for _, want := range []string{"=== QA ===", "keywords:", "[check]", "about 70 words", "below 50% to 70% utilisation", "Fortinet support"} {
		if !strings.Contains(br["brief"], want) {
			t.Errorf("brief misses %q", want)
		}
	}
	rec = do(h, tok, "POST", "/api/packs/"+p.ID+"/preview", map[string]string{"answer": claudeAnswer})
	var pv pack.Preview
	decode(t, rec, &pv)
	if !pv.OK || pv.Sections != 2 || pv.QA != 2 || pv.Checks != 1 {
		t.Fatalf("preview = %+v", pv)
	}
	if rec = do(h, tok, "POST", "/api/packs/"+p.ID+"/preview", map[string]string{"answer": "sorry, I can't"}); !strings.Contains(rec.Body.String(), `"ok":false`) {
		t.Fatalf("bad answer accepted: %s", rec.Body)
	}
	rec = do(h, tok, "POST", "/api/packs/"+p.ID+"/import", map[string]string{"answer": claudeAnswer})
	if rec.Code != 201 {
		t.Fatalf("import: %d %s", rec.Code, rec.Body)
	}
	var im struct {
		Script store.Script `json:"script"`
	}
	decode(t, rec, &im)
	if im.Script.Parsed.QAItems != 2 || im.Script.Parsed.Meta.Extra["pack"] != p.ID || im.Script.Parsed.Meta.Lang != "en" {
		t.Fatalf("script = %+v", im.Script.Parsed.Meta)
	}
	// re-import updates the same script
	rec = do(h, tok, "POST", "/api/packs/"+p.ID+"/import", map[string]string{"answer": strings.Replace(claudeAnswer, "over-prepare", "over-plan", 1)})
	var im2 struct {
		Script store.Script `json:"script"`
	}
	decode(t, rec, &im2)
	if im2.Script.ID != im.Script.ID || !strings.Contains(im2.Script.Source, "over-plan") {
		t.Fatalf("re-import made %s", im2.Script.ID)
	}

	// export to a zip and import it back as a second pack
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	rec = do(h, tok, "POST", "/api/packs/"+p.ID+"/export", nil)
	var ex map[string]string
	decode(t, rec, &ex)
	data, err := os.ReadFile(ex["path"])
	if err != nil {
		t.Fatal(err)
	}
	zr, _ := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	names := []string{}
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	if !strings.Contains(strings.Join(names, ","), "terrific-interview/script.md") {
		t.Fatalf("zip = %v", names)
	}
	req := httptest.NewRequest("POST", "/api/packs/import", bytes.NewReader(data))
	req.Header.Set("X-TC-Token", tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 201 || !strings.Contains(rr.Body.String(), `"terrific-interview-2"`) || !strings.Contains(rr.Body.String(), "scriptId") {
		t.Fatalf("zip import: %d %s", rr.Code, rr.Body)
	}
}

func TestLiveSession(t *testing.T) {
	// a fake whisper-server: returns a fixed transcript and checks the request shape
	var gotLang, gotPrompt string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			b, _ := io.ReadAll(part)
			switch part.FormName() {
			case "language":
				gotLang = string(b)
			case "prompt":
				gotPrompt = string(b)
			case "file":
				if !bytes.HasPrefix(b, []byte("RIFF")) {
					http.Error(w, "not wav", 400)
					return
				}
			}
		}
		w.Write([]byte(`{"text":" So, what would you say is your biggest weakness? "}`))
	}))
	defer fake.Close()

	s, h := newTest(t)
	s.Speech.Endpoint = fake.URL
	tok := s.Token
	rec := do(h, tok, "POST", "/api/packs", pack.Pack{Title: "p"})
	var p pack.Pack
	decode(t, rec, &p)
	rec = do(h, tok, "POST", "/api/packs/"+p.ID+"/import", map[string]string{"answer": claudeAnswer})
	var im struct {
		Script store.Script `json:"script"`
	}
	decode(t, rec, &im)

	rec = do(h, tok, "POST", "/api/live/start", map[string]any{"scriptId": im.Script.ID, "save": false})
	var st struct {
		ID string `json:"id"`
	}
	decode(t, rec, &st)

	// other side through audio
	req := httptest.NewRequest("POST", "/api/live/"+st.ID+"/audio?role=other&lang=en", bytes.NewReader(make([]byte, 32000)))
	req.Header.Set("X-TC-Token", tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "Q: What is your biggest weakness?") {
		t.Fatalf("audio: %d %s", rr.Code, rr.Body)
	}
	if gotLang != "en" || !strings.Contains(gotPrompt, "weakness") {
		t.Fatalf("whisper request lang=%q prompt=%q", gotLang, gotPrompt)
	}

	// me, through text, on the main track
	rec = do(h, tok, "POST", "/api/live/"+st.ID+"/text", map[string]any{"role": "me", "text": "hi I'm Jose I help customers adopt"})
	var res struct {
		Position struct {
			Para, InPara int
			Accepted     bool
		} `json:"position"`
	}
	decode(t, rec, &res)
	if !res.Position.Accepted || res.Position.Para != 0 || res.Position.InPara != 7 {
		t.Fatalf("position: %s", rec.Body)
	}
	// a Q&A track (section 3 = "Q: What is your biggest weakness?")
	rec = do(h, tok, "POST", "/api/live/"+st.ID+"/text", map[string]any{"role": "me", "text": "I over-prepare so I now", "track": 3})
	if !strings.Contains(rec.Body.String(), `"inPara":5`) {
		t.Fatalf("qa track: %s", rec.Body)
	}
	rec = do(h, tok, "POST", "/api/live/"+st.ID+"/stop", map[string]any{"used": []string{"Q: What is your biggest weakness?"}})
	var stop map[string]any
	decode(t, rec, &stop)
	notes, _ := stop["notes"].(string)
	if !strings.Contains(notes, "biggest weakness?") || !strings.Contains(notes, "## Sections you used") || stop["saved"] != "" || strings.Contains(notes, "## Transcript") {
		t.Fatalf("notes: %v", stop)
	}
	if rec = do(h, tok, "POST", "/api/live/"+st.ID+"/text", map[string]any{"role": "me", "text": "x"}); rec.Code != 404 {
		t.Fatalf("session should be gone: %d", rec.Code)
	}
}

func TestOpenAllowList(t *testing.T) {
	s, h := newTest(t)
	var opened string
	s.Open = func(u string) error { opened = u; return nil }
	if rec := do(h, s.Token, "POST", "/api/open", map[string]string{"url": "https://claude.ai/new"}); rec.Code != 200 || opened != "https://claude.ai/new" {
		t.Fatalf("claude: %d", rec.Code)
	}
	if rec := do(h, s.Token, "POST", "/api/open", map[string]string{"url": "file:///C:/Windows/system32/calc.exe"}); rec.Code != 400 {
		t.Fatalf("file url allowed: %d", rec.Code)
	}
}

func TestSpeechStatus(t *testing.T) {
	s, h := newTest(t)
	rec := do(h, s.Token, "GET", "/api/speech/status", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "base-q5_1") {
		t.Fatalf("status: %s", rec.Body)
	}
	if rec := do(h, s.Token, "POST", "/api/speech/install", map[string]string{"model": "nope"}); rec.Code != 409 {
		t.Fatalf("bad model: %d", rec.Code)
	}
}

func TestSettingsKeepSync(t *testing.T) {
	s, h := newTest(t)
	st := s.Store.Settings()
	st.Sync = true
	_, _ = s.Store.SaveSettings(st)
	st.Sync = false
	st.WPM = 150
	do(h, s.Token, "PUT", "/api/settings", st)
	if got := s.Store.Settings(); !got.Sync || got.WPM != 150 {
		t.Fatalf("settings PUT changed sync: %+v", got)
	}
}
