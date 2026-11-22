// Package speech runs local speech-to-text with whisper.cpp's whisper-server, which TeleCyborg
// starts on 127.0.0.1 and talks to over HTTP. Audio never leaves the computer
// (docs/requirements/10-privacy-and-consent.md).
package speech

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RuntimeVersion is the whisper.cpp release TeleCyborg ships and downloads.
const RuntimeVersion = "v1.8.4"

// RuntimeURL is the official Windows x64 CPU build of whisper.cpp.
const RuntimeURL = "https://github.com/ggml-org/whisper.cpp/releases/download/" + RuntimeVersion + "/whisper-bin-x64.zip"

// RuntimeFiles are the files TeleCyborg needs from the whisper.cpp release.
var RuntimeFiles = []string{"whisper-server.exe", "whisper.dll", "ggml.dll", "ggml-base.dll", "ggml-cpu.dll"}

// Model is a whisper model TeleCyborg can download.
type Model struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	File      string `json:"file"`
	SizeMB    int    `json:"sizeMB"`
	Note      string `json:"note"`
	Installed bool   `json:"installed"`
	SHA256    string `json:"-"` // checked after download when known
}

// Models available for download (multilingual: English, Spanish and Portuguese).
var Models = []Model{
	{ID: "base-q5_1", Label: "Base (recommended)", File: "ggml-base-q5_1.bin", SizeMB: 57, Note: "good accuracy, fast on most laptops",
		SHA256: "422f1ae452ade6f30a004d7e5c6a43195e4433bc370bf23fac9cc591f01a8898"},
	{ID: "tiny-q5_1", Label: "Tiny", File: "ggml-tiny-q5_1.bin", SizeMB: 31, Note: "fastest, for older computers; more mistakes"},
	{ID: "small-q5_1", Label: "Small", File: "ggml-small-q5_1.bin", SizeMB: 181, Note: "best accuracy; needs a fast CPU"},
}

// ModelURL is where a model is downloaded from (the whisper.cpp project's Hugging Face repo).
func ModelURL(file string) string {
	return "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/" + file
}

// Download describes a running or finished download.
type Download struct {
	What   string `json:"what"`
	Got    int64  `json:"got"`
	Total  int64  `json:"total"`
	Done   bool   `json:"done"`
	Error  string `json:"error,omitempty"`
	Active bool   `json:"active"`
}

// Status is what the UI shows about the engine.
type Status struct {
	Runtime       string    `json:"runtime"` // path of whisper-server, "" when missing
	RuntimeFrom   string    `json:"runtimeFrom"`
	Models        []Model   `json:"models"`
	Model         string    `json:"model"` // selected model id ("" when none installed)
	Running       bool      `json:"running"`
	Ready         bool      `json:"ready"` // runtime and model present
	External      bool      `json:"external"`
	Download      *Download `json:"download,omitempty"`
	LastError     string    `json:"lastError,omitempty"`
	NeedsVCRedist bool      `json:"needsVCRedist"`
}

// Manager owns the whisper-server process and the downloads.
type Manager struct {
	Dir    string // %APPDATA%\TeleCyborg\speech
	ExeDir string // folder of telecyborg.exe (the installer puts the runtime in ExeDir\whisper)
	// Endpoint, when set, is an already running whisper-server (tests, or TELECYBORG_WHISPER_URL).
	Endpoint string
	Client   *http.Client

	mu        sync.Mutex
	cmd       *proc
	url       string
	model     string // id of the model the process runs
	selected  string
	download  *Download
	lastError string
	starting  chan struct{}
}

// NewManager creates a manager with its folders.
func NewManager(dataDir, exeDir string) *Manager {
	m := &Manager{Dir: filepath.Join(dataDir, "speech"), ExeDir: exeDir, Endpoint: os.Getenv("TELECYBORG_WHISPER_URL"),
		Client: &http.Client{Timeout: 60 * time.Second}}
	_ = os.MkdirAll(filepath.Join(m.Dir, "models"), 0o755)
	return m
}

func exeName(n string) string {
	if runtime.GOOS != "windows" {
		return strings.TrimSuffix(n, ".exe")
	}
	return n
}

// RuntimePath finds whisper-server: next to TeleCyborg (installer), in the data folder
// (downloaded), or on PATH (development).
func (m *Manager) RuntimePath() (string, string) {
	name := exeName("whisper-server.exe")
	if m.ExeDir != "" {
		p := filepath.Join(m.ExeDir, "whisper", name)
		if fileExists(p) {
			return p, "bundled"
		}
	}
	p := filepath.Join(m.Dir, "runtime", name)
	if fileExists(p) {
		return p, "downloaded"
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, "path"
	}
	return "", ""
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func (m *Manager) modelPath(file string) string { return filepath.Join(m.Dir, "models", file) }

func findModel(id string) (Model, bool) {
	for _, md := range Models {
		if md.ID == id {
			return md, true
		}
	}
	return Model{}, false
}

// Select chooses the model to use (from settings).
func (m *Manager) Select(id string) {
	m.mu.Lock()
	m.selected = id
	m.mu.Unlock()
}

func (m *Manager) models() ([]Model, string) {
	out := make([]Model, len(Models))
	chosen := ""
	for i, md := range Models {
		md.Installed = fileExists(m.modelPath(md.File))
		out[i] = md
	}
	for _, md := range out {
		if md.Installed && md.ID == m.selected {
			chosen = md.ID
		}
	}
	if chosen == "" {
		for _, md := range out {
			if md.Installed {
				chosen = md.ID
				break
			}
		}
	}
	return out, chosen
}

// Status reports the engine state.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	rt, from := m.RuntimePath()
	ms, chosen := m.models()
	st := Status{Runtime: rt, RuntimeFrom: from, Models: ms, Model: chosen, Running: m.cmd != nil || m.Endpoint != "",
		External: m.Endpoint != "", LastError: m.lastError, NeedsVCRedist: needsVCRedist()}
	st.Ready = m.Endpoint != "" || (rt != "" && chosen != "")
	if m.download != nil {
		d := *m.download
		st.Download = &d
	}
	return st
}

// Install downloads what is missing (the runtime, then the model) in the background.
func (m *Manager) Install(modelID string) error {
	md, ok := findModel(modelID)
	if !ok {
		return fmt.Errorf("unknown model %q", modelID)
	}
	m.mu.Lock()
	if m.download != nil && m.download.Active {
		m.mu.Unlock()
		return errors.New("a download is already running")
	}
	m.download = &Download{What: "starting", Active: true}
	m.selected = modelID
	m.mu.Unlock()
	go func() {
		err := m.install(md)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.download.Active = false
		if err != nil {
			m.download.Error = err.Error()
			return
		}
		m.download.Done = true
		m.download.What = "ready"
	}()
	return nil
}

func (m *Manager) progress(what string, got, total int64) {
	m.mu.Lock()
	if m.download != nil {
		m.download.What, m.download.Got, m.download.Total = what, got, total
	}
	m.mu.Unlock()
}

func (m *Manager) install(md Model) error {
	if rt, _ := m.RuntimePath(); rt == "" {
		if runtime.GOOS != "windows" {
			return errors.New("whisper-server was not found; install whisper.cpp and put whisper-server on PATH")
		}
		if err := m.downloadRuntime(); err != nil {
			return fmt.Errorf("speech runtime: %w", err)
		}
	}
	dst := m.modelPath(md.File)
	if fileExists(dst) {
		return nil
	}
	return m.fetch("model "+md.Label, ModelURL(md.File), dst, func(p string) error {
		if err := checkModel(p); err != nil {
			return err
		}
		return checkSHA(p, md.SHA256)
	})
}

func (m *Manager) fetch(what, url, dst string, verify func(string) error) error {
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download failed (are you online?): %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	tmp := dst + ".part"
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	total := resp.ContentLength
	var got int64
	buf := make([]byte, 256<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				os.Remove(tmp)
				return werr
			}
			got += int64(n)
			m.progress(what, got, total)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			os.Remove(tmp)
			return fmt.Errorf("download interrupted: %w", rerr)
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if verify != nil {
		if err := verify(tmp); err != nil {
			os.Remove(tmp)
			return err
		}
	}
	return os.Rename(tmp, dst)
}

// checkModel verifies a ggml model file by its magic number.
func checkModel(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var magic [4]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil || string(magic[:]) != "lmgg" {
		return errors.New("this is not a whisper.cpp (ggml) model file")
	}
	return nil
}

func checkSHA(path, want string) error {
	if want == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return errors.New("the downloaded model is damaged (checksum mismatch); try again")
	}
	return nil
}

func (m *Manager) downloadRuntime() error {
	zipPath := filepath.Join(m.Dir, "whisper-bin-x64.zip")
	if err := m.fetch("speech runtime", RuntimeURL, zipPath, nil); err != nil {
		return err
	}
	defer os.Remove(zipPath)
	return ExtractRuntime(zipPath, filepath.Join(m.Dir, "runtime"))
}

// ExtractRuntime copies the needed whisper.cpp files out of the release zip into dst.
func ExtractRuntime(zipPath, dst string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	want := map[string]bool{}
	for _, f := range RuntimeFiles {
		want[strings.ToLower(f)] = true
	}
	found := 0
	for _, f := range zr.File {
		base := strings.ToLower(filepath.Base(strings.ReplaceAll(f.Name, "\\", "/")))
		if !want[base] {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(filepath.Join(dst, base))
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, io.LimitReader(rc, 64<<20))
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
		found++
	}
	if found < len(RuntimeFiles) {
		return fmt.Errorf("the whisper.cpp package is missing files (%d of %d found)", found, len(RuntimeFiles))
	}
	return nil
}

// ImportModel saves a model file the user already has.
func (m *Manager) ImportModel(r io.Reader, name string) (string, error) {
	file := ""
	for _, md := range Models {
		if strings.EqualFold(md.File, filepath.Base(name)) {
			file = md.File
		}
	}
	if file == "" {
		return "", fmt.Errorf("pick one of: %s", modelFiles())
	}
	tmp := m.modelPath(file) + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, io.LimitReader(r, 2<<30)); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	f.Close()
	if err := checkModel(tmp); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return file, os.Rename(tmp, m.modelPath(file))
}

func modelFiles() string {
	var s []string
	for _, md := range Models {
		s = append(s, md.File)
	}
	return strings.Join(s, ", ")
}

// ensure starts whisper-server with the selected model, or restarts it on a model change.
func (m *Manager) ensure(ctx context.Context) (string, error) {
	if m.Endpoint != "" {
		return strings.TrimRight(m.Endpoint, "/"), nil
	}
	m.mu.Lock()
	_, chosen := m.models()
	if m.cmd != nil && m.model == chosen {
		u := m.url
		m.mu.Unlock()
		return u, nil
	}
	if ch := m.starting; ch != nil {
		m.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return "", ctx.Err()
		}
		return m.ensure(ctx)
	}
	if m.cmd != nil {
		m.stopLocked()
	}
	rt, _ := m.RuntimePath()
	if rt == "" || chosen == "" {
		m.mu.Unlock()
		return "", errors.New("the speech engine is not installed yet")
	}
	md, _ := findModel(chosen)
	ch := make(chan struct{})
	m.starting = ch
	m.mu.Unlock()

	u, cmd, err := start(rt, m.modelPath(md.File))
	m.mu.Lock()
	m.starting = nil
	close(ch)
	if err != nil {
		m.lastError = err.Error()
		m.mu.Unlock()
		return "", err
	}
	m.cmd, m.url, m.model, m.lastError = cmd, u, chosen, ""
	m.mu.Unlock()
	go func() {
		<-cmd.done
		m.mu.Lock()
		if m.cmd == cmd {
			m.cmd = nil
		}
		m.mu.Unlock()
	}()
	return u, nil
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

type proc struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func (p *proc) gone() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

func start(rt, model string) (string, *proc, error) {
	port, err := freePort()
	if err != nil {
		return "", nil, err
	}
	threads := runtime.NumCPU() / 2
	if threads < 2 {
		threads = 2
	}
	if threads > 8 {
		threads = 8
	}
	var stderr bytes.Buffer
	cmd := exec.Command(rt, "-m", model, "--host", "127.0.0.1", "--port", strconv.Itoa(port), "-t", strconv.Itoa(threads))
	cmd.Dir = filepath.Dir(rt)
	cmd.Stdout = io.Discard
	lw := &limited{b: &stderr, n: 8 << 10}
	cmd.Stderr = lw
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return "", nil, fmt.Errorf("could not start the speech engine: %w", err)
	}
	bindToJob(cmd)
	p := &proc{cmd: cmd, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(p.done) }()
	u := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(60 * time.Second)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		if p.gone() {
			msg := strings.TrimSpace(lw.String())
			if len(msg) > 300 {
				msg = "…" + msg[len(msg)-300:]
			}
			if msg == "" {
				msg = "it closed right away"
			}
			return "", nil, fmt.Errorf("the speech engine did not start: %s. On a new PC, install the Microsoft Visual C++ runtime (vc_redist.x64.exe)", msg)
		}
		resp, err := client.Get(u + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return u, p, nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	return "", nil, errors.New("the speech engine took too long to load the model")
}

type limited struct {
	b  *bytes.Buffer
	n  int
	mu sync.Mutex
}

func (l *limited) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.b.Write(p)
	if l.b.Len() > l.n {
		rest := l.b.Bytes()[l.b.Len()-l.n:]
		nb := bytes.NewBuffer(append([]byte(nil), rest...))
		*l.b = *nb
	}
	return len(p), nil
}

func (l *limited) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// Stop ends the whisper-server process.
func (m *Manager) Stop() {
	m.mu.Lock()
	m.stopLocked()
	m.mu.Unlock()
}

func (m *Manager) stopLocked() {
	if m.cmd != nil && m.cmd.cmd.Process != nil {
		_ = m.cmd.cmd.Process.Kill()
	}
	m.cmd = nil
}

// Request is one transcription.
type Request struct {
	PCM    []byte // 16 kHz mono signed 16-bit little-endian
	Lang   string // "en", "es", "pt" or "auto"
	Prompt string // context that steers spelling (names, products)
}

var hallucinations = map[string]bool{
	"thank you.": true, "thanks for watching!": true, "thank you for watching.": true, "you": true,
	"[blank_audio]": true, "(silence)": true, "[silence]": true, "[music]": true, "(music)": true,
	"subtítulos realizados por la comunidad de amara.org": true, "gracias.": true, "obrigado.": true,
	"¡gracias!": true, "...": true,
}

// Transcribe sends audio to whisper-server and returns the text.
func (m *Manager) Transcribe(ctx context.Context, r Request) (string, error) {
	if len(r.PCM) < 3200 { // < 0.1 s
		return "", nil
	}
	u, err := m.ensure(ctx)
	if err != nil {
		return "", err
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "chunk.wav")
	_, _ = fw.Write(WAV(r.PCM, 16000))
	lang := r.Lang
	if lang == "" {
		lang = "auto"
	}
	fields := map[string]string{"response_format": "json", "temperature": "0.0", "temperature_inc": "0.2",
		"language": lang, "prompt": r.Prompt, "no_timestamps": "true", "suppress_nst": "true"}
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	mw.Close()
	req, _ := http.NewRequestWithContext(ctx, "POST", u+"/inference", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := m.Client.Do(req)
	if err != nil {
		m.mu.Lock()
		m.lastError = err.Error()
		if m.cmd != nil && m.cmd.gone() {
			m.cmd = nil
		}
		m.mu.Unlock()
		return "", fmt.Errorf("speech engine: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("speech engine: %s %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var out struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("speech engine sent an unexpected answer")
	}
	if out.Error != "" {
		return "", errors.New("speech engine: " + out.Error)
	}
	return Clean(out.Text), nil
}

// Clean trims whisper output and drops its typical hallucinations on silence.
func Clean(t string) string {
	t = strings.Join(strings.Fields(t), " ")
	if hallucinations[strings.ToLower(t)] {
		return ""
	}
	return t
}

// WAV wraps PCM16 mono samples in a WAV header.
func WAV(pcm []byte, rate int) []byte {
	var b bytes.Buffer
	b.Grow(44 + len(pcm))
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(36+len(pcm)))
	b.WriteString("WAVEfmt ")
	_ = binary.Write(&b, binary.LittleEndian, uint32(16))
	_ = binary.Write(&b, binary.LittleEndian, uint16(1)) // PCM
	_ = binary.Write(&b, binary.LittleEndian, uint16(1)) // mono
	_ = binary.Write(&b, binary.LittleEndian, uint32(rate))
	_ = binary.Write(&b, binary.LittleEndian, uint32(rate*2))
	_ = binary.Write(&b, binary.LittleEndian, uint16(2))
	_ = binary.Write(&b, binary.LittleEndian, uint16(16))
	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(pcm)))
	b.Write(pcm)
	return b.Bytes()
}
