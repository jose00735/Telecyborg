# 01 — Roadmap

| Version | Stage | Scope | Status |
|---|---|---|---|
| v0.1.0 | 1 | Teleprompter beta: library, editor, auto-scroll, fit-to-time, paragraph timing, pace coaching, rehearsal report, Matrix UI and transitions, always-on-top, mirror | released |
| v1.0.0 | 2 + 3 | Content packs (brief for Claude → pitch + Q&A), listening with local whisper.cpp, keyword cue suggestions, jump and "back to narrative", section search, live transcript, post-meeting notes, smart follow (clock / follow / hybrid), spoken pace, filler words and silences, answer-length guard, hide from screen share, Windows installer | **released** |
| v1.1.0 | — | Android app (standalone; library, editor, prompter, Q&A detours, packs, settings), PC as sync hub (TLS, pairing by code comparison, three-way sync), reading over your camera image on the PC, Macawlink phone-screen prompter | **released** |
| v1.2 | — | Listening on Android (on-device recognition), global hotkeys, history charts, cheat cards, multilingual sections, code signing | planned |

The original plan split stage 2 into v0.3 (packs) and v0.4 (listening) and stage 3 into v0.5.
They shipped together in v1.0.0 because listening, cues and follow share one audio pipeline.

## Milestone order for stages 2 and 3 (done in v1.0.0)
1. Script format: `keywords:`, `=== QA ===`, `# Q:` headings, `[check]` (04).
2. Text matching: normalisation, light EN/ES/PT stemming, synonyms (`internal/words`, `internal/cue`).
3. Alignment of speech to script (`internal/follow`).
4. Content packs: brief, validation, import, zip export/import (`internal/pack`).
5. Speech engine: whisper.cpp `whisper-server` manager, model download, process lifetime (`internal/speech`).
6. Live sessions and API (`internal/live`, `internal/server/stage2.go`).
7. UI: audio capture and chunking, listening dialog, suggestions, detours, search, follow, report.
8. Tests: unit tests per package, API tests with a fake whisper server, end-to-end UI test with a
   fake microphone (`tools/e2e_test.py`), and a run against a real whisper-server build.
9. Windows build, installer, GitHub release.

## Definition of done (every stage)
- Requirements in this folder updated to match what was built.
- `go vet` and `go test ./...` pass; UI end-to-end test passes.
- Windows exe built with `-trimpath`, tagged, and attached to a GitHub release (zipped).
