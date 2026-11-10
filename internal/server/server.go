// Package server serves the embedded UI and the JSON API on 127.0.0.1.
package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strings"

	"github.com/jose00735/telecyborg/internal/script"
	"github.com/jose00735/telecyborg/internal/store"
)

// Window is implemented by the native window (nil in browser mode).
type Window interface {
	SetOnTop(on bool) error
}

// Server is the local HTTP server.
type Server struct {
	Store   *store.Store
	UI      fs.FS
	Version string
	Token   string
	Window  Window
	Native  bool
	Quit    func()
	ln      net.Listener
}

// New creates a server with a random token.
func New(st *store.Store, ui fs.FS, version string) *Server {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return &Server{Store: st, UI: ui, Version: version, Token: hex.EncodeToString(b)}
}

// Listen binds 127.0.0.1 on the given port (0 = random) and returns the URL to open.
func (s *Server) Listen(port string) (string, error) {
	if port == "" {
		port = "0"
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		return "", err
	}
	s.ln = ln
	return "http://" + ln.Addr().String() + "/#t=" + s.Token, nil
}

// Serve blocks serving requests.
func (s *Server) Serve() error { return http.Serve(s.ln, s.Handler()) }

// Handler returns the full HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/", s.auth(http.HandlerFunc(s.api)))
	static := http.FileServer(http.FS(s.UI))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")
		static.ServeHTTP(w, r)
	})
	return mux
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("X-TC-Token")
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.Token)) != 1 {
			http.Error(w, "missing or wrong token", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(v)
}

func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/"), "/")
	parts := strings.Split(path, "/")
	switch {
	case path == "info" && r.Method == http.MethodGet:
		writeJSON(w, 200, map[string]any{"version": s.Version, "native": s.Native, "dir": s.Store.Dir})

	case path == "quit" && r.Method == http.MethodPost:
		writeJSON(w, 200, map[string]bool{"quitting": true})
		if s.Quit != nil {
			go s.Quit()
		}

	case path == "settings" && r.Method == http.MethodGet:
		writeJSON(w, 200, s.Store.Settings())
	case path == "settings" && r.Method == http.MethodPut:
		var st store.Settings
		if err := readJSON(r, &st); err != nil {
			fail(w, 400, "settings are not valid JSON")
			return
		}
		out, err := s.Store.SaveSettings(st)
		if err != nil {
			fail(w, 500, "could not save settings: "+err.Error())
			return
		}
		writeJSON(w, 200, out)

	case path == "parse" && r.Method == http.MethodPost:
		var in struct {
			Body string `json:"body"`
		}
		if err := readJSON(r, &in); err != nil {
			fail(w, 400, "body is not valid JSON")
			return
		}
		writeJSON(w, 200, script.Parse(in.Body))

	case path == "window/ontop" && r.Method == http.MethodPost:
		var in struct {
			On bool `json:"on"`
		}
		if err := readJSON(r, &in); err != nil {
			fail(w, 400, "body is not valid JSON")
			return
		}
		if s.Window == nil {
			fail(w, 409, "keep on top works in the TeleCyborg window, not in a browser")
			return
		}
		if err := s.Window.SetOnTop(in.On); err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"on": in.On})

	case path == "scripts" && r.Method == http.MethodGet:
		list, err := s.Store.List()
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, list)
	case path == "scripts" && r.Method == http.MethodPost:
		var in store.Input
		if err := readJSON(r, &in); err != nil {
			fail(w, 400, "script is not valid JSON")
			return
		}
		id, err := s.Store.Create(in)
		if err != nil {
			fail(w, 500, "could not save the script: "+err.Error())
			return
		}
		sc, _ := s.Store.Get(id)
		writeJSON(w, 201, sc)

	case len(parts) == 2 && parts[0] == "scripts":
		s.scriptItem(w, r, parts[1])
	case len(parts) == 3 && parts[0] == "scripts" && parts[2] == "rehearsals":
		s.rehearsals(w, r, parts[1])
	default:
		fail(w, 404, "unknown API route")
	}
}

func (s *Server) scriptItem(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodGet:
		sc, err := s.Store.Get(id)
		if err != nil {
			fail(w, 404, "script not found")
			return
		}
		writeJSON(w, 200, sc)
	case http.MethodPut:
		var in store.Input
		if err := readJSON(r, &in); err != nil {
			fail(w, 400, "script is not valid JSON")
			return
		}
		if err := s.Store.Save(id, in); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				fail(w, 404, "script not found")
				return
			}
			fail(w, 500, "could not save the script: "+err.Error())
			return
		}
		sc, _ := s.Store.Get(id)
		writeJSON(w, 200, sc)
	case http.MethodDelete:
		if err := s.Store.Delete(id); err != nil {
			fail(w, 404, "script not found")
			return
		}
		w.WriteHeader(204)
	default:
		fail(w, 405, "method not allowed")
	}
}

func (s *Server) rehearsals(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodGet:
		runs, err := s.Store.Runs(id)
		if err != nil {
			fail(w, 404, "script not found")
			return
		}
		writeJSON(w, 200, runs)
	case http.MethodPost:
		var run store.Rehearsal
		if err := readJSON(r, &run); err != nil {
			fail(w, 400, "rehearsal is not valid JSON")
			return
		}
		if err := s.Store.AddRun(id, run); err != nil {
			fail(w, 404, "script not found")
			return
		}
		writeJSON(w, 201, map[string]bool{"saved": true})
	default:
		fail(w, 405, "method not allowed")
	}
}
