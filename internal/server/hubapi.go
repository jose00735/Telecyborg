package server

import (
	"net/http"
	"strings"

	"github.com/jose00735/telecyborg/internal/hub"
	"github.com/jose00735/telecyborg/internal/macaw"
)

// hubAPI is the PC side of phone sync: turn it on, pair, forget.
func (s *Server) hubAPI(w http.ResponseWriter, r *http.Request, parts []string) {
	if s.Hub == nil {
		fail(w, 503, "phone sync is not available")
		return
	}
	route := strings.Join(parts, "/")
	enabled := s.Store.Settings().Sync
	switch {
	case route == "status" && r.Method == http.MethodGet:
		writeJSON(w, 200, s.Hub.Status(enabled))
	case route == "enable" && r.Method == http.MethodPost:
		var in struct {
			On bool `json:"on"`
		}
		if err := readJSON(r, &in); err != nil {
			fail(w, 400, "body is not valid JSON")
			return
		}
		st := s.Store.Settings()
		st.Sync = in.On
		_, _ = s.Store.SaveSettings(st)
		if in.On {
			if err := s.Hub.Start(); err != nil {
				fail(w, 500, err.Error())
				return
			}
		} else {
			s.Hub.Stop()
		}
		writeJSON(w, 200, s.Hub.Status(in.On))
	case route == "pair" && r.Method == http.MethodPost:
		if !enabled {
			st := s.Store.Settings()
			st.Sync = true
			_, _ = s.Store.SaveSettings(st)
		}
		if err := s.Hub.Start(); err != nil {
			fail(w, 500, err.Error())
			return
		}
		s.Hub.OpenPairing()
		writeJSON(w, 200, s.Hub.Status(true))
	case route == "pair/cancel" && r.Method == http.MethodPost:
		s.Hub.ClosePairing()
		writeJSON(w, 200, s.Hub.Status(enabled))
	case route == "decide" && r.Method == http.MethodPost:
		var in struct {
			OK bool `json:"ok"`
		}
		if err := readJSON(r, &in); err != nil {
			fail(w, 400, "body is not valid JSON")
			return
		}
		if err := s.Hub.Decide(in.OK); err != nil {
			fail(w, 409, err.Error())
			return
		}
		writeJSON(w, 200, s.Hub.Status(enabled))
	case route == "forget" && r.Method == http.MethodPost:
		var in struct {
			ID string `json:"id"`
		}
		if err := readJSON(r, &in); err != nil {
			fail(w, 400, "body is not valid JSON")
			return
		}
		s.Hub.Phones.Remove(in.ID)
		writeJSON(w, 200, s.Hub.Status(enabled))
	default:
		fail(w, 404, "unknown API route")
	}
}

var _ = hub.Port

// macawAPI forwards the prompter to Macawlink (phone camera preview overlay).
func (s *Server) macawAPI(w http.ResponseWriter, r *http.Request, parts []string) {
	route := strings.Join(parts, "/")
	switch {
	case route == "status" && r.Method == http.MethodGet:
		writeJSON(w, 200, s.Macaw.Status())
	case route == "script" && r.Method == http.MethodPost:
		var in macaw.Script
		if err := readJSON(r, &in); err != nil {
			fail(w, 400, "body is not valid JSON")
			return
		}
		if err := s.Macaw.SetScript(in); err != nil {
			fail(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	case route == "state" && r.Method == http.MethodPost:
		var in macaw.State
		if err := readJSON(r, &in); err != nil {
			fail(w, 400, "body is not valid JSON")
			return
		}
		s.Macaw.SetState(in)
		writeJSON(w, 200, map[string]bool{"ok": true})
	case route == "clear" && r.Method == http.MethodPost:
		_ = s.Macaw.Clear()
		writeJSON(w, 200, map[string]bool{"ok": true})
	case route == "keys" && r.Method == http.MethodGet:
		k := s.Macaw.Keys()
		if k == nil {
			k = []string{}
		}
		writeJSON(w, 200, map[string]any{"keys": k})
	default:
		fail(w, 404, "unknown API route")
	}
}
