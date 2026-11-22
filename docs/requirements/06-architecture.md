# 06 — Architecture

```
telecyborg.exe
├── main.go               flags, starts server, opens the window, stops the speech engine on exit
├── window_windows.go     native window (WebView2 via github.com/jchv/go-webview2, pure Go), DPI aware
├── window_other.go       non-Windows: opens the default browser
├── rsrc_windows_*.syso   icon and manifest (generated with akavel/rsrc from assets/)
├── internal/script       Markdown script parser (sections, keywords, Q&A, paragraphs, pauses, word counts)
├── internal/store        scripts, settings and rehearsal history on disk
├── internal/words        text folding, EN/ES/PT light stemming, fuzzy word match, filler words
├── internal/cue          keyword matcher: transcript of the other side → suggested sections
├── internal/follow       aligner: transcript of the speaker → position in a track of paragraphs
├── internal/speech       whisper.cpp whisper-server manager: runtime, models, downloads, transcription
├── internal/live         listening sessions: both sides, prompts, notes, optional saving
├── internal/pack         content packs: brief for Claude, validation, import, zip export/import
├── internal/server       local HTTP server: embedded UI + JSON API
├── internal/hub          phone sync hub: TLS server, pairing, manifest, three-way sync rules (13)
├── internal/mdns         DNS-SD responder (_telecyborg._tcp), adapted from Macawlink
├── internal/macaw        prompter feed to Macawlink's local API (14)
├── cmd/installer         Windows setup (per-user install, shortcuts, uninstaller)
├── tools/hubtest         the real hub on a temp folder, for the Android interop test
├── android/              Android app (Kotlin, Compose) and golden fixtures shared with Go
└── web/                  UI: HTML, CSS, vanilla JS (embedded with go:embed)
    ├── core.js           API token, helpers
    ├── audio.js          capture, resampling to 16 kHz, speech detection, chunking
    ├── pcm-worklet.js    AudioWorklet tap
    ├── packs.js          content pack screens
    ├── sync.js           phone sync dialog
    ├── live.js           listening dialog, engine install, chunk queues, transcript
    └── app.js            library, editor, prompter (tracks, cues, detours, follow), report
```

## Decisions
| ID | Decision | Why |
|---|---|---|
| AR-1 | Go backend + web UI rendered in WebView2. | Web tech for the Matrix look; Go for a single small exe. |
| AR-2 | Pure Go, `CGO_ENABLED=0`. | Cross-compiles from any OS; no C toolchain. |
| AR-3 | UI is vanilla JS/CSS, no build step. | Nothing to install; easy to change. |
| AR-4 | Server binds to `127.0.0.1` on a random free port. | Never reachable from the network. |
| AR-5 | Every API call needs a per-launch random token (header `X-TC-Token`). | Stops other local web pages from calling the API. |
| AR-6 | Scripts stored as Markdown, not a database. | Portable, readable, Git-friendly. |
| AR-7 | Timing engine runs in the UI (requestAnimationFrame). | Frame-accurate scrolling with no round trips. |
| AR-8 | Window-level features (on top, hide from capture) go through `/api/window/*` to Win32. | Keeps the UI unaware of the OS. |
| AR-9 | Speech-to-text is whisper.cpp's `whisper-server` (v1.8.4, CPU build) as a child process on 127.0.0.1, fed 16 kHz WAV chunks over HTTP. | Accurate in EN/ES/PT, offline, keeps the model loaded between chunks, and keeps TeleCyborg pure Go. |
| AR-10 | Audio is captured in the UI (getUserMedia for microphones and input devices, getDisplayMedia for system audio) and resampled and chunked in JS. | WebView2/Chromium already has device permission UI and system-audio loopback; no WASAPI code to maintain. |
| AR-11 | Speech detection is an adaptive energy gate in JS; only voiced chunks are sent. | Saves CPU and avoids whisper's hallucinations on silence. |
| AR-12 | The other side is cut at pauses (≤ 7 s); the speaker in 2.4 s windows with 0.6 s overlap. | Questions arrive whole; following stays responsive. The aligner drops the repeated overlap. |
| AR-13 | whisper-server runs in a Windows job object that kills it with TeleCyborg. | No orphaned engine after a crash. |
| AR-14 | Matching and alignment run in Go, per session. | Unit-testable; the UI only renders results. |
| AR-16 | The PC is the hub; phones sync with it over HTTPS on the LAN (47811) and work standalone. | One source of truth, no cloud, phones never talk to each other. |
| AR-17 | The Macawlink feed goes through Macawlink's existing local API and wire protocol. | One USB/Wi-Fi link to the phone; the text never enters the video. |
| AR-15 | The prompter has tracks: the narrative and one answer at a time, each on its own plan clock, with a return stack. | Detours do not disturb the narrative's timing. |

## API (v1.0.0)
| Method | Path | Purpose |
|---|---|---|
| GET | `/api/scripts` | list summaries (now with `qa` and `pack`) |
| POST | `/api/scripts` | create `{title, body, wpm, target}` → script |
| GET / PUT / DELETE | `/api/scripts/{id}` | script / save / delete |
| POST | `/api/parse` | parse unsaved text (editor live stats, outline) |
| GET / POST | `/api/scripts/{id}/rehearsals` | history / add run |
| GET / PUT | `/api/settings` | settings (now with `speech`) |
| POST | `/api/window/ontop`, `/api/window/capture` | keep on top; hide from screen capture |
| POST | `/api/open` | open an allow-listed https page (claude.ai, VC++ runtime) in the browser |
| GET | `/api/info` | version, native window or browser |
| GET | `/api/speech/status` | runtime, models, download progress, errors |
| POST | `/api/speech/install` | `{model}` download runtime (if missing) and model in the background |
| POST | `/api/speech/import?name=` | upload a model file the user already has |
| POST | `/api/speech/test?lang=` | transcribe raw PCM (engine test) |
| POST | `/api/live/start` | `{scriptId, save}` → session id and outline |
| POST | `/api/live/{id}/audio?role=&track=&lang=` | raw 16 kHz PCM16 → `{text, suggestions | position}` |
| POST | `/api/live/{id}/text` | same with text (typed questions, tests) |
| POST | `/api/live/{id}/seek` | `{track, para}` after a manual jump |
| POST | `/api/live/{id}/stop` | `{used}` → notes (saved only when the session saves) |
| GET / POST | `/api/packs` | list / create |
| GET / PUT / DELETE | `/api/packs/{id}` | pack |
| GET | `/api/packs/{id}/brief` | brief for Claude + pack.md |
| POST | `/api/packs/{id}/preview`, `/import` | validate / import Claude's answer as a script |
| POST | `/api/packs/{id}/export`, `/api/packs/import` | zip to Downloads / import a zip |

## API added in v1.1.0
| Method | Path | Purpose |
|---|---|---|
| GET | `/api/hub/status` | sync hub state, addresses, paired phones, pending pairing code |
| POST | `/api/hub/enable`, `/pair`, `/pair/cancel`, `/decide`, `/forget` | control the hub |
| GET | `/api/macaw/status` | is Macawlink running, does the phone support the prompter |
| POST | `/api/macaw/script`, `/state`, `/clear` | feed the phone |
| GET | `/api/macaw/keys` | taps made on the phone |

The phone-facing hub API is in 13-android-and-sync.md.
