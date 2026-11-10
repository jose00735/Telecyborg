# 01 — Roadmap

| Version | Stage | Scope | Output |
|---|---|---|---|
| v0.1.0 | 1 | Teleprompter beta: library, editor, auto-scroll, fit-to-time, paragraph timing, pace coaching, rehearsal report, Matrix UI and transitions, always-on-top, mirror | `telecyborg.exe` (windows/amd64) |
| v0.2.0 | 1 | Polish from beta feedback: compact "webcam strip" window mode, global hotkeys, phone remote over LAN, rehearsal history charts | exe |
| v0.3.0 | 2a | Content packs: topic brief → Markdown for Claude → import pitch + Q&A pack; cue index per section | exe + pack format |
| v0.4.0 | 2b | Listening: local speech-to-text of the other side; keyword spotting; suggestion chips; jump and "back to narrative" stack | exe (+ speech model download) |
| v0.5.0 | 3 | Smart follow: align your speech to the script; position-aware scrolling and pace coaching | exe |
| v1.0.0 | — | Stable release, installer optional, signed binary | exe + msi |

## Milestone order for stage 1 (done in v0.1.0)
1. Project definition and requirement docs.
2. Go backend: storage, script parser, local server, native window.
3. Web UI: library, editor, prompter, report, Matrix transitions.
4. Timing engine and pace coach.
5. Tests: parser unit tests, API tests, headless UI smoke test.
6. Windows amd64 build and GitHub release.

## Definition of done (every stage)
- Requirements in this folder updated to match what was built.
- `go vet` and `go test ./...` pass; UI smoke test passes.
- Windows exe built with `-trimpath`, tagged, and attached to a GitHub release.
