package hub

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/jose00735/telecyborg/internal/pack"
	"github.com/jose00735/telecyborg/internal/store"
)

// Item kinds that travel between the PC hub and phones.
const (
	KindScript     = "script"     // content: the Markdown source, verbatim
	KindPack       = "pack"       // content: {"pack": <pack.json verbatim>, "answer": "<answer.md>"}
	KindSettings   = "settings"   // id "prefs"; content: SyncedSettings JSON
	KindRehearsals = "rehearsals" // id = script id; content: JSON array; merged, never conflicts
)

// Entry is one line of the manifest.
type Entry struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Hash     string `json:"hash"`     // "" when deleted
	Modified int64  `json:"modified"` // unix ms
	Deleted  bool   `json:"deleted,omitempty"`
}

// Item is an entry with its content.
type Item struct {
	Entry
	Content string `json:"content,omitempty"`
}

// SyncedSettings are the preferences that follow you between PC and phone.
type SyncedSettings struct {
	WPM       int     `json:"wpm"`
	FontSize  int     `json:"fontSize"`
	Mirror    bool    `json:"mirror"`
	FocusLine float64 `json:"focusLine"`
	Countdown int     `json:"countdown"`
	Motion    string  `json:"motion"`
	AnswerMax int     `json:"answerMax"`
}

// packContent is the wire form of a pack: both files verbatim.
type packContent struct {
	Pack   json.RawMessage `json:"pack"`
	Answer string          `json:"answer"`
}

// Hash is the content hash both sides compute: hex SHA-256 of the content string.
func Hash(content string) string {
	s := sha256.Sum256([]byte(content))
	return hex.EncodeToString(s[:])
}

// Repo reads and writes the PC's data for sync.
type Repo struct {
	Scripts *store.Store
	Packs   *pack.Store
	dir     string
	mu      sync.Mutex
	tombs   map[string]Entry // key kind/id
}

func newRepo(dir string, st *store.Store, ps *pack.Store) *Repo {
	r := &Repo{Scripts: st, Packs: ps, dir: dir, tombs: map[string]Entry{}}
	if b, err := os.ReadFile(filepath.Join(dir, "tombstones.json")); err == nil {
		var list []Entry
		if json.Unmarshal(b, &list) == nil {
			for _, e := range list {
				r.tombs[e.Kind+"/"+e.ID] = e
			}
		}
	}
	return r
}

func (r *Repo) saveTombs() {
	list := make([]Entry, 0, len(r.tombs))
	for _, e := range r.tombs {
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Kind+list[i].ID < list[j].Kind+list[j].ID })
	b, _ := json.MarshalIndent(list, "", " ")
	_ = os.MkdirAll(r.dir, 0o700)
	_ = os.WriteFile(filepath.Join(r.dir, "tombstones.json"), b, 0o600)
}

// Deleted records that the PC deleted an item, so phones delete it too.
func (r *Repo) Deleted(kind, id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tombs[kind+"/"+id] = Entry{Kind: kind, ID: id, Deleted: true, Modified: time.Now().UnixMilli()}
	r.saveTombs()
}

func (r *Repo) clearTomb(kind, id string) {
	if _, ok := r.tombs[kind+"/"+id]; ok {
		delete(r.tombs, kind+"/"+id)
		r.saveTombs()
	}
}

// content returns the current content of an item ("" + false when absent).
func (r *Repo) content(kind, id string) (string, time.Time, bool) {
	switch kind {
	case KindScript:
		b, mod, err := r.Scripts.Raw(id)
		return string(b), mod, err == nil
	case KindPack:
		pj, ans, mod, err := r.Packs.Raw(id)
		if err != nil {
			return "", time.Time{}, false
		}
		b, _ := json.Marshal(packContent{Pack: compactJSON(pj), Answer: string(ans)})
		return string(b), mod, true
	case KindSettings:
		if id != "prefs" {
			return "", time.Time{}, false
		}
		st := r.Scripts.Settings()
		b, _ := json.Marshal(SyncedSettings{WPM: st.WPM, FontSize: st.FontSize, Mirror: st.Mirror, FocusLine: st.FocusLine,
			Countdown: st.Countdown, Motion: st.Motion, AnswerMax: st.Speech.AnswerMax})
		return string(b), r.Scripts.SettingsTime(), true
	case KindRehearsals:
		b, mod := r.Scripts.RunsRaw(id)
		if b == nil {
			return "", time.Time{}, false
		}
		var runs []store.Rehearsal
		if json.Unmarshal(b, &runs) != nil {
			return "", time.Time{}, false
		}
		out, _ := json.Marshal(runs)
		return string(out), mod, true
	}
	return "", time.Time{}, false
}

// compactJSON keeps the bytes valid as an embedded JSON value (pack.json is written indented).
func compactJSON(b []byte) json.RawMessage {
	var v any
	if json.Unmarshal(b, &v) != nil {
		return json.RawMessage(`{}`)
	}
	out, _ := json.Marshal(v)
	return out
}

// Manifest lists every item on the PC, deletions included.
func (r *Repo) Manifest() []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []Entry{}
	add := func(kind, id string) {
		c, mod, ok := r.content(kind, id)
		if !ok {
			return
		}
		r.clearTomb(kind, id)
		out = append(out, Entry{Kind: kind, ID: id, Hash: Hash(c), Modified: mod.UnixMilli()})
	}
	for _, id := range r.Scripts.IDs() {
		add(KindScript, id)
	}
	for _, id := range r.Packs.IDs() {
		add(KindPack, id)
	}
	for _, id := range r.Scripts.RunIDs() {
		add(KindRehearsals, id)
	}
	add(KindSettings, "prefs")
	for _, e := range r.tombs {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Get returns one item.
func (r *Repo) Get(kind, id string) (Item, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, mod, ok := r.content(kind, id)
	if !ok {
		if t, ok := r.tombs[kind+"/"+id]; ok {
			return Item{Entry: t}, true
		}
		return Item{}, false
	}
	return Item{Entry: Entry{Kind: kind, ID: id, Hash: Hash(c), Modified: mod.UnixMilli()}, Content: c}, true
}

// ErrConflict means the PC copy changed since the phone last saw it.
var ErrConflict = errors.New("changed on the PC since your last sync")

// Put stores an item from a phone. base is the hash the phone last saw ("" for a new item).
// Scripts and packs refuse to overwrite a different PC version (ErrConflict); rehearsals
// merge; settings take the phone's version.
func (r *Repo) Put(kind, id, base, content string) (Item, error) {
	if !store.ValidID(id) && !(kind == KindSettings && id == "prefs") {
		return Item{}, errors.New("invalid id")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, _, exists := r.content(kind, id)
	if exists && (kind == KindScript || kind == KindPack) && Hash(cur) != base && Hash(cur) != Hash(content) {
		return Item{}, ErrConflict
	}
	var err error
	switch kind {
	case KindScript:
		err = r.Scripts.Put(id, []byte(content))
	case KindPack:
		var pc packContent
		if err = json.Unmarshal([]byte(content), &pc); err == nil {
			err = r.Packs.PutRaw(id, pc.Pack, []byte(pc.Answer))
		}
	case KindSettings:
		var ss SyncedSettings
		if err = json.Unmarshal([]byte(content), &ss); err == nil {
			st := r.Scripts.Settings()
			st.WPM, st.FontSize, st.Mirror, st.FocusLine, st.Countdown, st.Motion = ss.WPM, ss.FontSize, ss.Mirror, ss.FocusLine, ss.Countdown, ss.Motion
			if ss.AnswerMax > 0 {
				st.Speech.AnswerMax = ss.AnswerMax
			}
			_, err = r.Scripts.SaveSettings(st)
		}
	case KindRehearsals:
		var runs []store.Rehearsal
		if err = json.Unmarshal([]byte(content), &runs); err == nil {
			err = r.Scripts.MergeRuns(id, runs)
		}
	default:
		err = errors.New("unknown kind")
	}
	if err != nil {
		return Item{}, err
	}
	r.clearTomb(kind, id)
	c, mod, _ := r.content(kind, id)
	return Item{Entry: Entry{Kind: kind, ID: id, Hash: Hash(c), Modified: mod.UnixMilli()}, Content: c}, nil
}

// Delete removes an item for a phone, if the PC still has the version the phone saw.
func (r *Repo) Delete(kind, id, base string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, _, exists := r.content(kind, id)
	if !exists {
		return nil
	}
	if Hash(cur) != base {
		return ErrConflict
	}
	var err error
	switch kind {
	case KindScript:
		err = r.Scripts.Delete(id)
	case KindPack:
		err = r.Packs.Delete(id)
	default:
		return errors.New("this kind cannot be deleted")
	}
	if err != nil {
		return err
	}
	r.tombs[kind+"/"+id] = Entry{Kind: kind, ID: id, Deleted: true, Modified: time.Now().UnixMilli()}
	r.saveTombs()
	return nil
}
