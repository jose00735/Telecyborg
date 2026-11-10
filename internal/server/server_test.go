package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/jose00735/telecyborg/internal/store"
)

func newTest(t *testing.T) (*Server, http.Handler) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := New(st, fstest.MapFS{"index.html": {Data: []byte("<h1>ui</h1>")}}, "test")
	return s, s.Handler()
}

func do(h http.Handler, token, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	if token != "" {
		req.Header.Set("X-TC-Token", token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestTokenRequired(t *testing.T) {
	s, h := newTest(t)
	if rec := do(h, "", "GET", "/api/scripts", nil); rec.Code != 403 {
		t.Fatalf("no token: %d", rec.Code)
	}
	if rec := do(h, "nope", "GET", "/api/scripts", nil); rec.Code != 403 {
		t.Fatalf("bad token: %d", rec.Code)
	}
	if rec := do(h, s.Token, "GET", "/api/scripts", nil); rec.Code != 200 {
		t.Fatalf("good token: %d", rec.Code)
	}
	if rec := do(h, "", "GET", "/", nil); rec.Code != 200 {
		t.Fatalf("static: %d", rec.Code)
	}
}

func TestScriptLifecycle(t *testing.T) {
	s, h := newTest(t)
	tok := s.Token
	rec := do(h, tok, "POST", "/api/scripts", store.Input{Title: "Pitch ñandú", Body: "# Intro\nHello there world.\n\n[pause 2]\n\nSecond part here.", WPM: 120, Target: 90})
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var sc store.Script
	_ = json.Unmarshal(rec.Body.Bytes(), &sc)
	if sc.ID != "pitch-nandu" || sc.Parsed.Words != 6 || sc.Parsed.Pauses != 2 || sc.Target != 90 || sc.WPM != 120 {
		t.Fatalf("created: %+v", sc.Summary)
	}

	rec = do(h, tok, "PUT", "/api/scripts/"+sc.ID, store.Input{Title: "Pitch ñandú", Body: "One two three.", WPM: 150})
	if rec.Code != 200 {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &sc)
	if sc.Words != 3 || sc.WPM != 150 || sc.Target != 0 {
		t.Fatalf("saved: %+v", sc.Summary)
	}

	run := store.Rehearsal{Planned: 1.2, Actual: 1.5, Completed: true, Paras: []store.ParaRun{{Planned: 1.2, Actual: 1.5}}}
	if rec = do(h, tok, "POST", "/api/scripts/"+sc.ID+"/rehearsals", run); rec.Code != 201 {
		t.Fatalf("add run: %d", rec.Code)
	}
	rec = do(h, tok, "GET", "/api/scripts", nil)
	var list []store.Summary
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 || list[0].Runs != 1 {
		t.Fatalf("list: %+v", list)
	}

	if rec = do(h, tok, "DELETE", "/api/scripts/"+sc.ID, nil); rec.Code != 204 {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec = do(h, tok, "GET", "/api/scripts/"+sc.ID, nil); rec.Code != 404 {
		t.Fatalf("after delete: %d", rec.Code)
	}
}

func TestBadIDs(t *testing.T) {
	s, h := newTest(t)
	for _, id := range []string{"..%2fsettings", "..", "UPPER", "a%20b", "x.md"} {
		rec := do(h, s.Token, "GET", "/api/scripts/"+id, nil)
		// ".." is cleaned by the router (redirect); everything else must be a plain 404
		if rec.Code != 404 && !(id == ".." && rec.Code == 301) {
			t.Errorf("id %q: %d", id, rec.Code)
		}
	}
}

func TestImportKeepsFrontMatter(t *testing.T) {
	s, h := newTest(t)
	rec := do(h, s.Token, "POST", "/api/scripts", store.Input{Body: "---\ntitle: From file\nwpm: 170\ntarget: 2:00\n---\nHello."})
	var sc store.Script
	_ = json.Unmarshal(rec.Body.Bytes(), &sc)
	if sc.Title != "From file" || sc.WPM != 170 || sc.Target != 120 {
		t.Fatalf("import: %+v", sc.Summary)
	}
}

func TestSettings(t *testing.T) {
	s, h := newTest(t)
	rec := do(h, s.Token, "PUT", "/api/settings", store.Settings{WPM: 5000, FontSize: 50, FocusLine: 0.9})
	var st store.Settings
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if st.WPM != 300 || st.FontSize != 50 || st.FocusLine != 0.38 {
		t.Fatalf("settings: %+v", st)
	}
}
