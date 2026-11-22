// Package hub makes the PC the hub of a hub-and-spoke sync with the TeleCyborg Android app.
// Phones work on their own and sync scripts, content packs, settings and rehearsal history
// with the PC when they are on the same network (docs/requirements/13-android-and-sync.md).
//
// Transport: HTTPS (TLS 1.3, self-signed ECDSA certificate that phones pin) on port 47811,
// advertised over mDNS as _telecyborg._tcp. Pairing is numeric comparison with commitment,
// like Macawlink: both screens show a 6-digit code and both users confirm.
package hub

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jose00735/telecyborg/internal/mdns"
	"github.com/jose00735/telecyborg/internal/pack"
	"github.com/jose00735/telecyborg/internal/store"
)

// Port is the hub's TLS port.
const Port = 47811

// PairWindow is how long "Pair a phone" stays open on the PC.
const PairWindow = 3 * time.Minute

// Pending is a pairing in progress.
type Pending struct {
	ID        string    `json:"id"`
	PhoneName string    `json:"phoneName"`
	Code      string    `json:"code,omitempty"` // shown once the phone sent its nonce
	State     string    `json:"state"`          // waiting-phone, compare, accepted, rejected
	Expires   time.Time `json:"expires"`

	phoneID  string
	pcNonce  []byte
	decision chan bool
	pcOK     *bool
}

// Status is what the PC UI shows.
type Status struct {
	Enabled   bool     `json:"enabled"`
	Running   bool     `json:"running"`
	Port      int      `json:"port"`
	Addresses []string `json:"addresses"`
	PCName    string   `json:"pcName"`
	Short     string   `json:"fingerprint"`
	Phones    []Phone  `json:"phones"`
	Pairing   bool     `json:"pairing"` // the pair window is open
	PairUntil string   `json:"pairUntil,omitempty"`
	Pending   *Pending `json:"pending,omitempty"`
	Error     string   `json:"error,omitempty"`
	LastSync  string   `json:"lastSync,omitempty"`
}

// Hub is the sync server.
type Hub struct {
	Repo   *Repo
	ID     *Identity
	Phones *Phones
	PCName string
	Addr   string // listen address, default ":47811"

	mu        sync.Mutex
	srv       *http.Server
	ln        net.Listener
	mdns      *mdns.Responder
	pairUntil time.Time
	pending   *Pending
	err       string
	lastSync  time.Time
	// OnChange is called after a phone changed data on the PC (to refresh the UI).
	OnChange func()
}

// New prepares a hub that keeps its state in dataDir/sync.
func New(dataDir string, st *store.Store, ps *pack.Store) (*Hub, error) {
	dir := dataDirSync(dataDir)
	host, _ := os.Hostname()
	if host == "" {
		host = "PC"
	}
	id, err := LoadOrCreateIdentity(dir, host)
	if err != nil {
		return nil, err
	}
	return &Hub{Repo: newRepo(dir, st, ps), ID: id, Phones: openPhones(dir), PCName: host, Addr: ":" + strconv.Itoa(Port)}, nil
}

func dataDirSync(d string) string { return filepath.Join(d, "sync") }

// Start listens on the LAN and advertises the hub.
func (h *Hub) Start() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.srv != nil {
		return nil
	}
	ln, err := net.Listen("tcp", h.Addr)
	if err != nil {
		h.err = "could not open port " + strconv.Itoa(Port) + ": " + err.Error()
		return err
	}
	h.err = ""
	h.ln = ln
	h.srv = &http.Server{Handler: h.Handler(), TLSConfig: h.ID.TLSConfig(), ReadHeaderTimeout: 10 * time.Second}
	srv := h.srv
	go func() {
		if err := srv.ServeTLS(ln, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Println("sync hub:", err)
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	h.mdns = mdns.Start(mdns.Service{Instance: "TeleCyborg on " + h.PCName, Host: mdns.HostLabel(h.PCName), Port: port,
		TXT: map[string]string{"v": "1", "name": h.PCName, "fp": h.ID.ShortFP()}})
	return nil
}

// Stop closes the listener.
func (h *Hub) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = h.srv.Shutdown(ctx)
		cancel()
		h.srv = nil
	}
	if h.mdns != nil {
		h.mdns.Stop()
		h.mdns = nil
	}
}

// Port returns the port in use (0 when stopped).
func (h *Hub) port() int {
	if h.ln == nil || h.srv == nil {
		return 0
	}
	return h.ln.Addr().(*net.TCPAddr).Port
}

// Status for the PC UI.
func (h *Hub) Status(enabled bool) Status {
	h.mu.Lock()
	defer h.mu.Unlock()
	st := Status{Enabled: enabled, Running: h.srv != nil, Port: h.port(), PCName: h.PCName, Short: h.ID.ShortFP(),
		Phones: h.Phones.List(), Error: h.err, Addresses: []string{}}
	if st.Port == 0 {
		st.Port = Port
	}
	for _, ip := range lanIPs() {
		st.Addresses = append(st.Addresses, net.JoinHostPort(ip, strconv.Itoa(st.Port)))
	}
	if time.Now().Before(h.pairUntil) {
		st.Pairing = true
		st.PairUntil = h.pairUntil.Format(time.RFC3339)
	}
	if h.pending != nil && time.Now().Before(h.pending.Expires) {
		p := *h.pending
		st.Pending = &p
	}
	if !h.lastSync.IsZero() {
		st.LastSync = h.lastSync.Format(time.RFC3339)
	}
	return st
}

func lanIPs() []string {
	out := []string{}
	ifs, _ := net.Interfaces()
	for _, ifi := range ifs {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifi.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil && !ipn.IP.IsLinkLocalUnicast() {
				out = append(out, ipn.IP.String())
			}
		}
	}
	return out
}

// OpenPairing lets one phone pair during the next PairWindow.
func (h *Hub) OpenPairing() {
	h.mu.Lock()
	h.pairUntil = time.Now().Add(PairWindow)
	h.mu.Unlock()
}

// ClosePairing stops accepting new pairings.
func (h *Hub) ClosePairing() {
	h.mu.Lock()
	h.pairUntil = time.Time{}
	if h.pending != nil && h.pending.decision != nil && h.pending.State == "compare" {
		select {
		case h.pending.decision <- false:
		default:
		}
	}
	h.pending = nil
	h.mu.Unlock()
}

// Decide is the PC user's answer to the code comparison.
func (h *Hub) Decide(ok bool) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	p := h.pending
	if p == nil || p.State != "compare" {
		return errors.New("no pairing is waiting for you")
	}
	select {
	case p.decision <- ok:
	default:
	}
	if ok {
		p.State = "accepted"
	} else {
		p.State = "rejected"
	}
	return nil
}

// ---------- HTTPS API for phones ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 16<<20)).Decode(v)
}

// Handler serves /v1/... to phones.
func (h *Hub) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/hello", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"app": "telecyborg", "v": 1, "pc_name": h.PCName, "fp": h.ID.ShortFP()})
	})
	mux.HandleFunc("POST /v1/pair/begin", h.pairBegin)
	mux.HandleFunc("POST /v1/pair/nonce", h.pairNonce)
	mux.HandleFunc("POST /v1/pair/confirm", h.pairConfirm)
	mux.Handle("GET /v1/manifest", h.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"pc_name": h.PCName, "now": time.Now().UnixMilli(), "items": h.Repo.Manifest()})
	})))
	mux.Handle("GET /v1/item", h.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		it, ok := h.Repo.Get(r.URL.Query().Get("kind"), r.URL.Query().Get("id"))
		if !ok {
			fail(w, 404, "not found")
			return
		}
		writeJSON(w, 200, it)
	})))
	mux.Handle("PUT /v1/item", h.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Kind, ID, Base, Content string
		}
		if err := readJSON(r, &in); err != nil {
			fail(w, 400, "bad JSON")
			return
		}
		it, err := h.Repo.Put(in.Kind, in.ID, in.Base, in.Content)
		if errors.Is(err, ErrConflict) {
			cur, _ := h.Repo.Get(in.Kind, in.ID)
			writeJSON(w, 409, map[string]any{"error": err.Error(), "current": cur})
			return
		}
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		h.changed()
		writeJSON(w, 200, it)
	})))
	mux.Handle("DELETE /v1/item", h.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		err := h.Repo.Delete(q.Get("kind"), q.Get("id"), q.Get("base"))
		if errors.Is(err, ErrConflict) {
			cur, _ := h.Repo.Get(q.Get("kind"), q.Get("id"))
			writeJSON(w, 409, map[string]any{"error": err.Error(), "current": cur})
			return
		}
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		h.changed()
		writeJSON(w, 200, map[string]bool{"deleted": true})
	})))
	return mux
}

func (h *Hub) changed() {
	if h.OnChange != nil {
		h.OnChange()
	}
}

func (h *Hub) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !h.Phones.Check(r.Header.Get("X-TC-Phone"), tok) {
			fail(w, 401, "this phone is not paired with this PC")
			return
		}
		h.mu.Lock()
		h.lastSync = time.Now()
		h.mu.Unlock()
		next.ServeHTTP(w, r)
	})
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *Hub) pairBegin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PhoneID string `json:"phone_id"`
		Name    string `json:"name"`
	}
	if err := readJSON(r, &in); err != nil || in.PhoneID == "" {
		fail(w, 400, "bad request")
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !time.Now().Before(h.pairUntil) {
		fail(w, 403, "On the PC, open TeleCyborg → Phone sync → Pair a phone, then try again.")
		return
	}
	if h.pending != nil && time.Now().Before(h.pending.Expires) && h.pending.State == "compare" {
		fail(w, 409, "another phone is pairing right now")
		return
	}
	nonce := make([]byte, 32)
	_, _ = rand.Read(nonce)
	name := strings.TrimSpace(in.Name)
	if len(name) > 60 {
		name = name[:60]
	}
	if name == "" {
		name = "Android phone"
	}
	h.pending = &Pending{ID: randomHex(8), PhoneName: name, State: "waiting-phone", Expires: time.Now().Add(2 * time.Minute),
		phoneID: in.PhoneID, pcNonce: nonce, decision: make(chan bool, 1)}
	writeJSON(w, 200, map[string]string{"pair_id": h.pending.ID, "commit": Commit(nonce), "pc_name": h.PCName})
}

func (h *Hub) pairNonce(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PairID string `json:"pair_id"`
		Nonce  string `json:"nonce"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "bad request")
		return
	}
	phoneNonce, err := hex.DecodeString(in.Nonce)
	if err != nil || len(phoneNonce) != 32 {
		fail(w, 400, "bad nonce")
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	p := h.pending
	if p == nil || subtle.ConstantTimeCompare([]byte(p.ID), []byte(in.PairID)) != 1 || p.State != "waiting-phone" || time.Now().After(p.Expires) {
		fail(w, 410, "pairing expired; start again")
		return
	}
	p.Code = SAS(h.ID.Fingerprint, p.pcNonce, phoneNonce)
	p.State = "compare"
	writeJSON(w, 200, map[string]string{"nonce": hex.EncodeToString(p.pcNonce)})
}

// pairConfirm waits (up to 2 minutes) for the PC user's decision.
func (h *Hub) pairConfirm(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PairID string `json:"pair_id"`
		OK     bool   `json:"ok"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, 400, "bad request")
		return
	}
	h.mu.Lock()
	p := h.pending
	if p == nil || p.ID != in.PairID || (p.State != "compare" && p.State != "accepted" && p.State != "rejected") {
		h.mu.Unlock()
		fail(w, 410, "pairing expired; start again")
		return
	}
	if !in.OK {
		h.pending = nil
		h.mu.Unlock()
		writeJSON(w, 200, map[string]any{"ok": false, "reason": "rejected on the phone"})
		return
	}
	ch := p.decision
	h.mu.Unlock()
	var pcOK bool
	select {
	case pcOK = <-ch:
	case <-time.After(2 * time.Minute):
	case <-r.Context().Done():
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pending = nil
	if !pcOK {
		writeJSON(w, 200, map[string]any{"ok": false, "reason": "rejected on the PC"})
		return
	}
	tok, err := h.Phones.Add(p.phoneID, p.PhoneName)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	h.pairUntil = time.Time{}
	writeJSON(w, 200, map[string]any{"ok": true, "token": tok, "pc_name": h.PCName})
}
