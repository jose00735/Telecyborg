// Package macaw feeds the running teleprompter to Macawlink (github.com/jose00735/macawlink),
// which shows it over the camera preview on the phone that is your webcam. TeleCyborg talks to
// Macawlink's local settings API on the same PC (127.0.0.1:47801); Macawlink forwards to the
// phone over its USB/Wi-Fi link. The text never enters the video stream.
package macaw

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

// DefaultURL is Macawlink's local settings API.
const DefaultURL = "http://127.0.0.1:47801"

// Block is one line of the prompter: a section heading, a paragraph or a cue.
type Block struct {
	Kind string `json:"k"` // sec, para, cue
	Text string `json:"t"` // plain text; **word** marks stress
}

// Script is what the phone shows.
type Script struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Track     string  `json:"track"` // main or answer
	Blocks    []Block `json:"blocks"`
	FontScale float64 `json:"font_scale"` // 1 = phone default
	Mirror    bool    `json:"mirror"`
}

// State is where the speaker is, sent several times a second.
type State struct {
	ID      string  `json:"id"`
	Para    int     `json:"para"`  // index among the "para" blocks
	Frac    float64 `json:"frac"`  // 0..1 inside it
	Speed   float64 `json:"speed"` // frac per second, for smooth motion between updates
	Playing bool    `json:"playing"`
	WPM     int     `json:"wpm"`
	Clock   string  `json:"clock"` // "0:42 / 2:00"
	Alert   string  `json:"alert"` // "", behind, ahead, miss, wrap
	Hint    string  `json:"hint,omitempty"`
}

// Status of the Macawlink connection.
type Status struct {
	Available bool   `json:"available"` // Macawlink is running on this PC
	Prompter  bool   `json:"prompter"`  // and supports the prompter feed
	Phone     bool   `json:"phone"`     // a phone is streaming
	Device    string `json:"device,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Feed sends prompter data to Macawlink.
type Feed struct {
	URL    string
	client *http.Client

	mu      sync.Mutex
	status  Status
	checked time.Time
	pending *State
	sending bool
}

// New creates a feed (url "" = DefaultURL).
func New(url string) *Feed {
	if url == "" {
		url = DefaultURL
	}
	return &Feed{URL: url, client: &http.Client{Timeout: 1500 * time.Millisecond}}
}

func (f *Feed) do(method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, f.URL+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("X-Macawlink", "1")
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == 404 {
		return errNoPrompter
	}
	if resp.StatusCode >= 300 {
		var e struct{ Error string }
		_ = json.Unmarshal(data, &e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return errors.New(e.Error)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

var errNoPrompter = errors.New("this Macawlink version has no prompter feed; update Macawlink to v0.4 or later")

// Status probes Macawlink (cached for 2 s).
func (f *Feed) Status() Status {
	f.mu.Lock()
	if time.Since(f.checked) < 2*time.Second {
		st := f.status
		f.mu.Unlock()
		return st
	}
	f.mu.Unlock()
	var out struct {
		Available bool   `json:"available"`
		Phone     bool   `json:"phone"`
		Device    string `json:"device"`
	}
	st := Status{}
	err := f.do("GET", "/api/prompter", nil, &out)
	switch {
	case err == nil:
		st = Status{Available: true, Prompter: true, Phone: out.Phone, Device: out.Device}
	case errors.Is(err, errNoPrompter):
		st = Status{Available: true, Error: err.Error()}
	default:
		st = Status{Error: "Macawlink is not running on this PC"}
	}
	f.mu.Lock()
	f.status, f.checked = st, time.Now()
	f.mu.Unlock()
	return st
}

// SetScript sends the text to show.
func (f *Feed) SetScript(s Script) error { return f.do("POST", "/api/prompter/script", s, nil) }

// SetState sends the position. Calls never queue up: only the newest state is sent.
func (f *Feed) SetState(s State) {
	f.mu.Lock()
	f.pending = &s
	if f.sending {
		f.mu.Unlock()
		return
	}
	f.sending = true
	f.mu.Unlock()
	go func() {
		for {
			f.mu.Lock()
			p := f.pending
			f.pending = nil
			if p == nil {
				f.sending = false
				f.mu.Unlock()
				return
			}
			f.mu.Unlock()
			_ = f.do("POST", "/api/prompter/state", p, nil)
		}
	}()
}

// Clear hides the prompter on the phone.
func (f *Feed) Clear() error { return f.do("POST", "/api/prompter/clear", nil, nil) }

// Keys returns taps from the phone since the last call (toggle, next, prev, faster, slower).
func (f *Feed) Keys() []string {
	var out struct {
		Keys []string `json:"keys"`
	}
	if f.do("GET", "/api/prompter/keys", nil, &out) != nil {
		return nil
	}
	return out.Keys
}
