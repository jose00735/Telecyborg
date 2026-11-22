package hub

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jose00735/telecyborg/internal/pack"
	"github.com/jose00735/telecyborg/internal/store"
)

type phone struct {
	t      *testing.T
	base   string
	c      *http.Client
	id     string
	token  string
	seenFP []byte
}

func (p *phone) do(method, path string, body any, out any) int {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, p.base+path, &buf)
	req.Header.Set("X-TC-Phone", p.id)
	req.Header.Set("Authorization", "Bearer "+p.token)
	resp, err := p.c.Do(req)
	if err != nil {
		p.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		s := sha256.Sum256(resp.TLS.PeerCertificates[0].Raw)
		p.seenFP = s[:]
	}
	return resp.StatusCode
}

func setup(t *testing.T) (*Hub, *phone, *store.Store, *pack.Store) {
	dir := t.TempDir()
	st, _ := store.Open(dir)
	ps, _ := pack.Open(dir)
	h, err := New(dir, st, ps)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(h.Handler())
	srv.TLS = h.ID.TLSConfig()
	srv.StartTLS()
	t.Cleanup(srv.Close)
	c := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13}}, Timeout: 10 * time.Second}
	return h, &phone{t: t, base: srv.URL, c: c, id: "phone-1"}, st, ps
}

func pair(t *testing.T, h *Hub, p *phone, accept bool) bool {
	var begin struct{ Pair_ID, Commit string }
	if code := p.do("POST", "/v1/pair/begin", map[string]string{"phone_id": p.id, "name": "Pixel"}, &begin); code != 200 {
		t.Fatalf("begin: %d", code)
	}
	phoneNonce := bytes.Repeat([]byte{7}, 32)
	var rev struct{ Nonce string }
	p.do("POST", "/v1/pair/nonce", map[string]string{"pair_id": begin.Pair_ID, "nonce": hex.EncodeToString(phoneNonce)}, &rev)
	pcNonce, _ := hex.DecodeString(rev.Nonce)
	if Commit(pcNonce) != begin.Commit {
		t.Fatal("commitment does not match the revealed nonce")
	}
	phoneCode := SAS(p.seenFP, pcNonce, phoneNonce)
	st := h.Status(true)
	if st.Pending == nil || st.Pending.Code != phoneCode || st.Pending.PhoneName != "Pixel" {
		t.Fatalf("codes differ: pc %+v phone %s", st.Pending, phoneCode)
	}
	go func() { time.Sleep(100 * time.Millisecond); _ = h.Decide(accept) }()
	var res struct {
		OK    bool
		Token string
	}
	p.do("POST", "/v1/pair/confirm", map[string]any{"pair_id": begin.Pair_ID, "ok": true}, &res)
	p.token = res.Token
	return res.OK
}

func TestPairingNeedsWindowAndBothSides(t *testing.T) {
	h, p, _, _ := setup(t)
	if code := p.do("POST", "/v1/pair/begin", map[string]string{"phone_id": p.id}, nil); code != 403 {
		t.Fatalf("pairing without the PC window: %d", code)
	}
	h.OpenPairing()
	if pair(t, h, p, false) {
		t.Fatal("paired although the PC rejected")
	}
	if code := p.do("GET", "/v1/manifest", nil, nil); code != 401 {
		t.Fatalf("unpaired manifest: %d", code)
	}
	if !pair(t, h, p, true) || p.token == "" {
		t.Fatal("pairing failed")
	}
	if code := p.do("GET", "/v1/manifest", nil, nil); code != 200 {
		t.Fatalf("paired manifest: %d", code)
	}
	if h.Status(true).Pairing {
		t.Fatal("pair window should close after a successful pairing")
	}
}

func TestSyncItems(t *testing.T) {
	h, p, st, ps := setup(t)
	h.OpenPairing()
	pair(t, h, p, true)
	id, _ := st.Create(store.Input{Title: "PC pitch", Body: "# Hi\nHello."})
	pk, _ := ps.Save(pack.Pack{Title: "Interview"}, store.Slug)

	var man struct{ Items []Entry }
	p.do("GET", "/v1/manifest", nil, &man)
	kinds := map[string]Entry{}
	for _, e := range man.Items {
		kinds[e.Kind+"/"+e.ID] = e
	}
	if _, ok := kinds["script/"+id]; !ok {
		t.Fatalf("manifest misses the script: %+v", man.Items)
	}
	if _, ok := kinds["pack/"+pk.ID]; !ok || kinds["settings/prefs"].Hash == "" {
		t.Fatalf("manifest misses pack or settings: %+v", man.Items)
	}
	// pull a script, edit it on the phone, push with the right base
	var it Item
	p.do("GET", "/v1/item?kind=script&id="+id, nil, &it)
	if Hash(it.Content) != it.Hash || !strings.Contains(it.Content, "Hello.") {
		t.Fatalf("item = %+v", it)
	}
	edited := strings.Replace(it.Content, "Hello.", "Hello from the phone.", 1)
	var put Item
	if code := p.do("PUT", "/v1/item", map[string]string{"kind": "script", "id": id, "base": it.Hash, "content": edited}, &put); code != 200 || put.Hash != Hash(edited) {
		t.Fatalf("push: %d %+v", code, put)
	}
	raw, _, _ := st.Raw(id)
	if string(raw) != edited {
		t.Fatal("PC did not store the phone's version verbatim")
	}
	// the PC changes it again; a phone push with the old base conflicts
	_ = st.Put(id, []byte(edited+"\nPC line.\n"))
	if code := p.do("PUT", "/v1/item", map[string]string{"kind": "script", "id": id, "base": put.Hash, "content": edited + "\nPhone line.\n"}, nil); code != 409 {
		t.Fatalf("conflict not detected: %d", code)
	}
	// new script from the phone
	if code := p.do("PUT", "/v1/item", map[string]string{"kind": "script", "id": "phone-pitch", "content": "---\ntitle: Phone pitch\n---\n# A\nText.\n"}, nil); code != 200 {
		t.Fatalf("new script: %d", code)
	}
	if s, err := st.Get("phone-pitch"); err != nil || s.Title != "Phone pitch" {
		t.Fatalf("phone script on PC: %v %+v", err, s.Summary)
	}
	// pack round trip keeps the answer
	var pit Item
	p.do("GET", "/v1/item?kind=pack&id="+pk.ID, nil, &pit)
	var pc packContent
	_ = json.Unmarshal([]byte(pit.Content), &pc)
	pc.Answer = "```markdown\n# Pitch\n```"
	b, _ := json.Marshal(pc)
	if code := p.do("PUT", "/v1/item", map[string]string{"kind": "pack", "id": pk.ID, "base": pit.Hash, "content": string(b)}, nil); code != 200 {
		t.Fatalf("pack push: %d", code)
	}
	if _, ans, _, _ := ps.Raw(pk.ID); !strings.Contains(string(ans), "# Pitch") {
		t.Fatal("answer not stored")
	}
	// settings from the phone
	if code := p.do("PUT", "/v1/item", map[string]string{"kind": "settings", "id": "prefs", "content": `{"wpm":155,"fontSize":52,"mirror":true,"focusLine":0.4,"countdown":2,"motion":"reduced","answerMax":75}`}, nil); code != 200 {
		t.Fatalf("settings push: %d", code)
	}
	if s := st.Settings(); s.WPM != 155 || !s.Mirror || s.Speech.AnswerMax != 75 {
		t.Fatalf("settings = %+v", s)
	}
	// rehearsals merge instead of overwrite
	_ = st.AddRun(id, store.Rehearsal{At: "2026-11-20T10:00:00Z", Actual: 60})
	rb, _ := json.Marshal([]store.Rehearsal{{At: "2026-11-21T09:00:00Z", Actual: 70}})
	p.do("PUT", "/v1/item", map[string]string{"kind": "rehearsals", "id": id, "content": string(rb)}, nil)
	if runs, _ := st.Runs(id); len(runs) != 2 || runs[0].At != "2026-11-21T09:00:00Z" {
		t.Fatalf("runs = %+v", runs)
	}
	// delete from the phone with a stale base conflicts, with the right base works and leaves a tombstone
	var cur Item
	p.do("GET", "/v1/item?kind=script&id=phone-pitch", nil, &cur)
	if code := p.do("DELETE", "/v1/item?kind=script&id=phone-pitch&base=nope", nil, nil); code != 409 {
		t.Fatalf("stale delete: %d", code)
	}
	if code := p.do("DELETE", "/v1/item?kind=script&id=phone-pitch&base="+cur.Hash, nil, nil); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	p.do("GET", "/v1/manifest", nil, &man)
	found := false
	for _, e := range man.Items {
		if e.ID == "phone-pitch" && e.Deleted {
			found = true
		}
	}
	if !found {
		t.Fatal("no tombstone for the deleted script")
	}
	// PC-side delete recorded through Deleted()
	_ = st.Delete(id)
	h.Repo.Deleted(KindScript, id)
	p.do("GET", "/v1/manifest", nil, &man)
	for _, e := range man.Items {
		if e.ID == id && e.Kind == KindScript && !e.Deleted {
			t.Fatal("PC delete not in manifest")
		}
	}
	_ = fmt.Sprint()
}
