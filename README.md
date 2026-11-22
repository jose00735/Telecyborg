# TeleCyborg

A teleprompter for interviews and customer meetings, built in Go with a web UI that looks like
the Matrix. It scrolls your script at the pace you choose, times every paragraph, and tells you
when to speed up so you finish on time.

It is also a live co-pilot: it listens to the other side of the call, suggests the prepared
answer to what you were just asked, lets you jump there and come back to your narrative, and
can follow your voice so the script moves with you. Speech recognition runs on your computer
(whisper.cpp); no audio leaves it.

> Status: **v1.0.0** — stages 1, 2 and 3. Windows 10/11 installer (x64) and portable builds.

## Stages

| Stage | What you get | Status |
|---|---|---|
| 1. Teleprompter | Script library, paste/import text, auto-scroll at a set pace or fitted to a target time, live paragraph timing, "speed up" coaching, rehearsal reports, Matrix UI | **done** (v0.1.0) |
| 2. Cue suggestions | Content packs (brief for Claude → pitch + likely questions), offline listening, keyword suggestions, jump to an answer and back to the narrative, Ctrl+K search, live transcript, answer-length guard, meeting notes | **done** (v1.0.0) |
| 3. Smart follow | Your voice aligned to the script: follow and hybrid scroll modes, spoken pace, off-script hold, filler words and long silences in the report | **done** (v1.0.0) |

## Install (Windows)

1. Download `TeleCyborg-v1.0.0-windows-amd64-setup.zip` from
   [Releases](https://github.com/jose00735/telecyborg/releases), unzip it and run
   `TeleCyborg-Setup-v1.0.0.exe`.
2. SmartScreen may warn because the app is not code-signed yet: **More info → Run anyway**.
3. Setup installs for your user only (no admin) with Start menu and desktop shortcuts and an
   uninstaller in Settings → Apps. If the Microsoft Visual C++ runtime is missing it offers to
   install it (the speech engine needs it).

No install wanted? Use the portable zip (`telecyborg.exe` + `whisper\` folder). ARM PCs have
their own portable zip. TeleCyborg needs the Microsoft Edge WebView2 runtime, which ships with
Windows 10 and 11; without it TeleCyborg opens in your browser.

## Use it

**Rehearse a script:** New script → paste or drop a `.md` / `.txt` file → set words per minute
or a target time (`2:00`) → **Rehearse**. Read along the bright focus line.

**Build a pitch with Claude (content packs):** Content packs → New pack → fill in the occasion,
audience, topics, facts and sources → **Save & write brief** → **Copy brief** → paste it into
Claude → paste Claude's answer back → **Create script**. You get a pitch timed to your target and
the likely questions with short answers; facts Claude could not confirm are marked `[check]`.

**Listen during a call:** in the prompter press **L**. The first time, download the speech model
(Base, 57 MB, once). Choose what to listen to:
- *The other side* — **System audio** (share your entire screen and tick "Share system audio";
  the picture is ignored) or an input device such as Stereo Mix or a virtual cable.
- *Me* — your microphone, for smart follow and filler words.
- *Scroll mode* — Clock, Follow my voice, or Hybrid.

When they ask something your script answers, a suggestion appears at the top right: press **1**
to jump to it, **Backspace** to return to where you were. Recording or transcribing other people
may need their consent — that is your responsibility (see
[10-privacy-and-consent.md](docs/requirements/10-privacy-and-consent.md)).

| Key | Action |
|---|---|
| Space | Play / pause |
| ↑ / ↓ | Faster / slower (5 wpm) |
| → / ← or Page Down / Page Up | Next / previous paragraph (presentation clickers work) |
| A | Apply the suggested speed when you are behind |
| L | Listen / stop listening |
| F | Scroll mode: clock → follow my voice → hybrid |
| 1 2 3 | Jump to a suggestion |
| Backspace | Back to the narrative |
| Ctrl+K or / | Find a section or answer, or type what you were asked |
| C | Show / hide the live transcript |
| H | Hide the window from screen sharing |
| T | Keep window on top |
| M | Mirror text (for beam-splitter glass) |
| + / − | Bigger / smaller text |
| Home | Restart |
| Esc | Stop (report and meeting notes) |

Scripts are plain Markdown in `%APPDATA%\TeleCyborg\scripts`; see
[04-script-format.md](docs/requirements/04-script-format.md) for `keywords:`, `=== QA ===` and
`# Q:` answers.

## Build

```
./build.sh v1.0.0
```

Runs `go vet` and the tests, fetches the whisper.cpp runtime, and writes the installer zip,
portable zips and checksums to `dist/`. Pure Go, no C compiler. A plain exe:

```
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -H windowsgui" -o telecyborg.exe .
```

On Linux and macOS the same code runs in your browser (`go run .`); put whisper.cpp's
`whisper-server` on PATH for listening. `python3 tools/e2e_test.py` runs the end-to-end UI test
with a fake microphone and a fake speech engine.

## Documentation

- [00-vision.md](docs/requirements/00-vision.md) — what TeleCyborg is and for whom
- [01-roadmap.md](docs/requirements/01-roadmap.md) — stages, milestones and releases
- [02-stage1-teleprompter.md](docs/requirements/02-stage1-teleprompter.md) — stage 1 requirements
- [03-pacing-and-timing.md](docs/requirements/03-pacing-and-timing.md) — the timing model and pace coaching
- [04-script-format.md](docs/requirements/04-script-format.md) — the Markdown script format
- [05-ui-matrix-design.md](docs/requirements/05-ui-matrix-design.md) — visual language, screens and transitions
- [06-architecture.md](docs/requirements/06-architecture.md) — Go backend, web UI, speech engine, API
- [07-stage2-cue-suggestions.md](docs/requirements/07-stage2-cue-suggestions.md) — listening, keyword cues, jump and return
- [08-content-packs.md](docs/requirements/08-content-packs.md) — topic packs and the brief for Claude
- [09-stage3-smart-follow.md](docs/requirements/09-stage3-smart-follow.md) — voice-following mode
- [10-privacy-and-consent.md](docs/requirements/10-privacy-and-consent.md) — recording, consent and data handling
- [11-build-and-release.md](docs/requirements/11-build-and-release.md) — builds, installer and releases
- [12-ideas-backlog.md](docs/requirements/12-ideas-backlog.md) — ideas beyond the three stages

Third-party software and licences: [THIRD_PARTY_NOTICES.txt](THIRD_PARTY_NOTICES.txt)
(whisper.cpp, go-webview2, Go).

## How this was made

Idea, requirements and testing by Jose Acosta Hernandez. Most of the code and documentation was
written by Claude (Anthropic's AI), directed by Jose.
