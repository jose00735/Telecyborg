package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jose00735/telecyborg/internal/pack"
	"github.com/jose00735/telecyborg/internal/speech"
	"github.com/jose00735/telecyborg/internal/store"
)

// Pages TeleCyborg may open in the browser.
var openAllowed = []string{"https://claude.ai/", "https://aka.ms/vs/17/release/vc_redist.x64.exe",
	"https://github.com/jose00735/telecyborg", "https://learn.microsoft.com/", "https://vb-audio.com/Cable/"}

func (s *Server) open(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL string `json:"url"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "body is not valid JSON")
		return
	}
	ok := false
	for _, p := range openAllowed {
		if strings.HasPrefix(in.URL, p) {
			ok = true
		}
	}
	if !ok || strings.ContainsAny(in.URL, " \"'<>\n") {
		fail(w, 400, "TeleCyborg only opens its own help pages and claude.ai")
		return
	}
	if s.Open == nil {
		fail(w, 409, "opening pages is not available")
		return
	}
	if err := s.Open(in.URL); err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"opened": true})
}

// ---------- speech engine ----------

func (s *Server) speechAPI(w http.ResponseWriter, r *http.Request, parts []string) {
	route := strings.Join(parts, "/")
	switch {
	case route == "status" && r.Method == http.MethodGet:
		writeJSON(w, 200, s.Speech.Status())
	case route == "install" && r.Method == http.MethodPost:
		var in struct {
			Model string `json:"model"`
		}
		if err := readJSON(r, &in); err != nil {
			fail(w, 400, "body is not valid JSON")
			return
		}
		if err := s.Speech.Install(in.Model); err != nil {
			fail(w, 409, err.Error())
			return
		}
		st := s.Store.Settings()
		st.Speech.Model = in.Model
		_, _ = s.Store.SaveSettings(st)
		writeJSON(w, 202, s.Speech.Status())
	case route == "import" && r.Method == http.MethodPost:
		name := r.URL.Query().Get("name")
		file, err := s.Speech.ImportModel(http.MaxBytesReader(w, r.Body, 2<<30), name)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"file": file, "status": s.Speech.Status()})
	case route == "stop" && r.Method == http.MethodPost:
		s.Speech.Stop()
		writeJSON(w, 200, s.Speech.Status())
	case route == "test" && r.Method == http.MethodPost:
		pcm, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
		if err != nil {
			fail(w, 413, "audio too long")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()
		t0 := time.Now()
		text, err := s.Speech.Transcribe(ctx, speechReq(pcm, r.URL.Query().Get("lang"), ""))
		if err != nil {
			fail(w, 503, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"text": text, "ms": time.Since(t0).Milliseconds()})
	default:
		fail(w, 404, "unknown API route")
	}
}

// ---------- live sessions ----------

func (s *Server) liveAPI(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 1 && parts[0] == "start" && r.Method == http.MethodPost {
		var in struct {
			ScriptID string `json:"scriptId"`
			Save     bool   `json:"save"`
		}
		if err := readJSON(r, &in); err != nil {
			fail(w, 400, "body is not valid JSON")
			return
		}
		sc, err := s.Store.Get(in.ScriptID)
		if err != nil {
			fail(w, 404, "script not found")
			return
		}
		sess := s.Live.Start(sc.ID, sc.Title, sc.Parsed, in.Save)
		writeJSON(w, 201, map[string]any{"id": sess.ID, "outline": sc.Parsed.Outline})
		return
	}
	if len(parts) != 2 || r.Method != http.MethodPost {
		fail(w, 404, "unknown API route")
		return
	}
	sess := s.Live.Get(parts[0])
	if sess == nil {
		fail(w, 404, "listening session ended")
		return
	}
	switch parts[1] {
	case "audio":
		q := r.URL.Query()
		role := q.Get("role")
		if role != "me" {
			role = "other"
		}
		track, _ := strconv.Atoi(q.Get("track"))
		if q.Get("track") == "" {
			track = -1
		}
		lang := q.Get("lang")
		pcm, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
		if err != nil {
			fail(w, 413, "audio chunk too long")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		text, err := s.Speech.Transcribe(ctx, speechReq(pcm, lang, sess.PromptFor(role, track, lang)))
		if err != nil {
			fail(w, 503, err.Error())
			return
		}
		writeJSON(w, 200, sess.Hear(role, text, track))
	case "text":
		var in struct {
			Role  string `json:"role"`
			Text  string `json:"text"`
			Track *int   `json:"track"`
		}
		if err := readJSON(r, &in); err != nil {
			fail(w, 400, "body is not valid JSON")
			return
		}
		if in.Role != "me" {
			in.Role = "other"
		}
		track := -1
		if in.Track != nil {
			track = *in.Track
		}
		writeJSON(w, 200, sess.Hear(in.Role, strings.TrimSpace(in.Text), track))
	case "seek":
		var in struct {
			Track int `json:"track"`
			Para  int `json:"para"`
		}
		if err := readJSON(r, &in); err != nil {
			fail(w, 400, "body is not valid JSON")
			return
		}
		writeJSON(w, 200, sess.Seek(in.Track, in.Para))
	case "stop":
		var in struct {
			Used []string `json:"used"`
		}
		_ = readJSON(r, &in)
		notes := sess.Notes(in.Used)
		path, err := sess.SaveNotes(s.Store.Dir, notes)
		s.Live.End(sess.ID)
		out := map[string]any{"notes": notes, "saved": path, "fillers": sess.Fillers()}
		if err != nil {
			out["error"] = "could not save the transcript: " + err.Error()
		}
		writeJSON(w, 200, out)
	default:
		fail(w, 404, "unknown API route")
	}
}

// ---------- content packs ----------

func (s *Server) packsAPI(w http.ResponseWriter, r *http.Request, parts []string) {
	switch {
	case len(parts) == 0 && r.Method == http.MethodGet:
		list, err := s.Packs.List()
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, list)
	case len(parts) == 0 && r.Method == http.MethodPost:
		var p pack.Pack
		if err := readJSON(r, &p); err != nil {
			fail(w, 400, "pack is not valid JSON")
			return
		}
		p.ID = ""
		out, err := s.Packs.Save(p, store.Slug)
		if err != nil {
			fail(w, 500, "could not save the pack: "+err.Error())
			return
		}
		writeJSON(w, 201, out)
	case len(parts) == 1 && parts[0] == "import" && r.Method == http.MethodPost:
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32<<20))
		if err != nil {
			fail(w, 413, "pack too large")
			return
		}
		im, err := s.Packs.ImportZip(data, store.Slug)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		out := map[string]any{"pack": im.Pack}
		if strings.TrimSpace(im.Script) != "" {
			id, err := s.Store.Create(store.Input{Body: pack.WithPack(im.Script, im.Pack)})
			if err == nil {
				_ = s.Packs.SetScript(im.Pack.ID, id, im.Script)
				im.Pack.ScriptID = id
				out["pack"] = im.Pack
				out["scriptId"] = id
			}
		}
		writeJSON(w, 201, out)
	case len(parts) == 1:
		s.packItem(w, r, parts[0])
	case len(parts) == 2:
		s.packAction(w, r, parts[0], parts[1])
	default:
		fail(w, 404, "unknown API route")
	}
}

func (s *Server) packItem(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodGet:
		p, err := s.Packs.Get(id)
		if err != nil {
			fail(w, 404, "pack not found")
			return
		}
		writeJSON(w, 200, p)
	case http.MethodPut:
		var p pack.Pack
		if err := readJSON(r, &p); err != nil {
			fail(w, 400, "pack is not valid JSON")
			return
		}
		p.ID = id
		out, err := s.Packs.Save(p, store.Slug)
		if err != nil {
			if errors.Is(err, pack.ErrNotFound) {
				fail(w, 404, "pack not found")
				return
			}
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, out)
	case http.MethodDelete:
		if err := s.Packs.Delete(id); err != nil {
			fail(w, 404, "pack not found")
			return
		}
		w.WriteHeader(204)
	default:
		fail(w, 405, "method not allowed")
	}
}

func (s *Server) packAction(w http.ResponseWriter, r *http.Request, id, action string) {
	p, err := s.Packs.Get(id)
	if err != nil {
		fail(w, 404, "pack not found")
		return
	}
	switch {
	case action == "brief" && r.Method == http.MethodGet:
		writeJSON(w, 200, map[string]string{"brief": pack.Brief(p), "markdown": pack.Markdown(p)})
	case action == "preview" && r.Method == http.MethodPost:
		var in struct {
			Answer string `json:"answer"`
		}
		if err := readJSON(r, &in); err != nil {
			fail(w, 400, "body is not valid JSON")
			return
		}
		writeJSON(w, 200, pack.Validate(in.Answer, p))
	case action == "import" && r.Method == http.MethodPost:
		var in struct {
			Answer string `json:"answer"`
		}
		if err := readJSON(r, &in); err != nil {
			fail(w, 400, "body is not valid JSON")
			return
		}
		pv := pack.Validate(in.Answer, p)
		if !pv.OK {
			writeJSON(w, 422, map[string]any{"error": strings.Join(pv.Errors, " "), "preview": pv})
			return
		}
		body := pack.WithPack(pv.Body, p)
		var sid string
		if p.ScriptID != "" {
			if err := s.Store.Replace(p.ScriptID, body); err == nil {
				sid = p.ScriptID
			} else if !errors.Is(err, store.ErrNotFound) {
				fail(w, 500, "could not save the script: "+err.Error())
				return
			}
		}
		if sid == "" {
			sid, err = s.Store.Create(store.Input{Body: body})
			if err != nil {
				fail(w, 500, "could not save the script: "+err.Error())
				return
			}
		}
		_ = s.Packs.SetScript(p.ID, sid, in.Answer)
		sc, _ := s.Store.Get(sid)
		writeJSON(w, 201, map[string]any{"script": sc, "preview": pv})
	case action == "export" && r.Method == http.MethodPost:
		src := ""
		if p.ScriptID != "" {
			if sc, err := s.Store.Get(p.ScriptID); err == nil {
				src = sc.Source
			}
		}
		data, err := s.Packs.Export(id, src)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		dir := downloadsDir()
		path := filepath.Join(dir, id+".telecyborg-pack.zip")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			fail(w, 500, "could not write the zip: "+err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"path": path})
	default:
		fail(w, 404, "unknown API route")
	}
}

func speechReq(pcm []byte, lang, prompt string) speech.Request {
	switch lang {
	case "en", "es", "pt", "auto":
	default:
		lang = "auto"
	}
	return speech.Request{PCM: pcm, Lang: lang, Prompt: prompt}
}

func downloadsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return os.TempDir()
	}
	d := filepath.Join(home, "Downloads")
	if st, err := os.Stat(d); err == nil && st.IsDir() {
		return d
	}
	return home
}
