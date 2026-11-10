# TeleCyborg

A teleprompter for interviews and customer meetings, built in Go with a web UI that looks like
the Matrix. It scrolls your script at the pace you choose, times every paragraph while you speak,
and tells you when to speed up so you finish on time.

Later stages turn it into a live co-pilot: it listens to the conversation, suggests the section
that answers what you were just asked, lets you jump there and return to your narrative, and
eventually follows your voice to know exactly where you are.

> Status: **v0.1.0 beta (stage 1)** — Windows amd64 single executable.

## Stages

| Stage | What you get | Status |
|---|---|---|
| 1. Teleprompter | Script library, paste/import text, auto-scroll at a set pace or fitted to a target time, live paragraph timing, "speed up" coaching, rehearsal reports, Matrix UI | **in beta** |
| 2. Cue suggestions | Content packs (topics, sources → Markdown brief for Claude → pitch + likely questions), live keyword spotting, "jump to answer" suggestions and "back to narrative" | planned |
| 3. Smart follow | Speech recognition aligns your voice to the script, so scrolling and pace coaching follow you instead of a clock | planned |

## Use it (stage 1)

1. Download `telecyborg.exe` from Releases and run it. No install. It needs the Microsoft Edge
   WebView2 runtime, which ships with Windows 10 and 11; if it is missing, TeleCyborg opens in your
   browser instead.
2. **New script** → paste or drop a `.md` / `.txt` file → set words per minute or a target time
   (for example `2:00`) → **Rehearse**.
3. Read along the bright focus line.

| Key | Action |
|---|---|
| Space | Play / pause |
| ↑ / ↓ | Faster / slower (5 wpm) |
| → / ← or Page Down / Page Up | Next / previous paragraph (presentation clickers work) |
| A | Apply the suggested speed when you are behind |
| Home | Restart |
| + / − | Bigger / smaller text |
| M | Mirror text (for beam-splitter glass) |
| T | Keep window on top |
| Esc | Back |

Scripts are plain Markdown files in `%APPDATA%\TeleCyborg\scripts`, so you can edit them with
any editor or keep them in Git. See [docs/requirements/04-script-format.md](docs/requirements/04-script-format.md).

## Build

```
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -H windowsgui" -o telecyborg.exe .
```

Pure Go, no C compiler. On Linux and macOS the same code runs in your browser (`go run .`),
which is handy for development.

## Documentation

- [docs/requirements/00-vision.md](docs/requirements/00-vision.md) — what TeleCyborg is and for whom
- [docs/requirements/01-roadmap.md](docs/requirements/01-roadmap.md) — stages, milestones and release plan
- [docs/requirements/02-stage1-teleprompter.md](docs/requirements/02-stage1-teleprompter.md) — stage 1 requirements (implemented)
- [docs/requirements/03-pacing-and-timing.md](docs/requirements/03-pacing-and-timing.md) — the timing model and pace coaching
- [docs/requirements/04-script-format.md](docs/requirements/04-script-format.md) — the Markdown script format
- [docs/requirements/05-ui-matrix-design.md](docs/requirements/05-ui-matrix-design.md) — visual language, screens and transitions
- [docs/requirements/06-architecture.md](docs/requirements/06-architecture.md) — Go backend, web UI, storage, security
- [docs/requirements/07-stage2-cue-suggestions.md](docs/requirements/07-stage2-cue-suggestions.md) — listening, keyword cues, jump and return
- [docs/requirements/08-content-packs.md](docs/requirements/08-content-packs.md) — topic packs and the Markdown brief for Claude
- [docs/requirements/09-stage3-smart-follow.md](docs/requirements/09-stage3-smart-follow.md) — voice-following mode
- [docs/requirements/10-privacy-and-consent.md](docs/requirements/10-privacy-and-consent.md) — recording, consent and data handling
- [docs/requirements/11-build-and-release.md](docs/requirements/11-build-and-release.md) — builds, versions and releases
- [docs/requirements/12-ideas-backlog.md](docs/requirements/12-ideas-backlog.md) — ideas beyond the three stages

## How this was made

Idea, requirements and testing by Jose Acosta Hernandez. Most of the code and documentation was
written by Claude (Anthropic's AI), directed by Jose.
