# 06 — Architecture

```
telecyborg.exe
├── main.go               flags, starts server, opens the window
├── window_windows.go     native window (WebView2 via github.com/jchv/go-webview2, pure Go)
├── window_other.go       non-Windows: opens the default browser
├── internal/script       Markdown script parser (sections, paragraphs, pauses, word counts)
├── internal/store        scripts, settings and rehearsal history on disk
├── internal/server       local HTTP server: embedded UI + JSON API
└── web/                  UI: HTML, CSS, vanilla JS (embedded with go:embed)
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
| AR-8 | Window-level features (always on top) go through a small `/api/window/*` bridge to Win32 `SetWindowPos`. | Keeps the UI unaware of the OS. |

## API (v0.1.0)
| Method | Path | Purpose |
|---|---|---|
| GET | `/api/scripts` | list summaries |
| POST | `/api/scripts` | create `{title, body, wpm, target}` → summary |
| GET | `/api/scripts/{id}` | raw script + parsed paragraphs |
| PUT | `/api/scripts/{id}` | save |
| DELETE | `/api/scripts/{id}` | delete |
| POST | `/api/parse` | parse unsaved text (editor live stats) |
| GET / POST | `/api/scripts/{id}/rehearsals` | history / add run |
| GET / PUT | `/api/settings` | settings |
| POST | `/api/window/ontop` | `{on: bool}` (native window only) |
| GET | `/api/info` | version, native window or browser |

## Later stages
- Stage 2 adds an audio pipeline (`internal/listen`): capture → local speech-to-text → keyword
  matcher → server-sent events to the UI. Speech engine options are compared in 07.
- Stage 3 adds `internal/follow`: forced alignment of the speaker's transcript to the script clock.
