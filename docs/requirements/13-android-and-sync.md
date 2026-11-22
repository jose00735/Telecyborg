# 13 — Android app and phone sync (v1.1.0)

TeleCyborg runs on Android as its own app (`android/`, Kotlin + Jetpack Compose). The phone
works **standalone**; the PC is the **hub** of a hub-and-spoke sync, so every phone exchanges
data with the PC and never with another phone directly.

## Android app
| ID | Requirement | Status |
|---|---|---|
| AN-1 | Library: scripts with words, pace, Q&A count, target, runs; new, edit, import `.md`, sample. | Done |
| AN-2 | Editor: title, Markdown body, pace slider, target and fit to time, stats, `[check]` count, sections with cue keywords, delete. | Done |
| AN-3 | Prompter: plan clock and pace coaching identical to the PC (03); tap to play/pause, swipe for next/previous paragraph, ±5 wpm, text size, mirror, timeline, countdown, screen kept on. | Done |
| AN-4 | Q&A answers open as a detour with their own clock and answer-length guard; "‹ back" returns to the narrative. | Done |
| AN-5 | Rehearsal report and history (runs carry `"device":"phone"`). | Done |
| AN-6 | Content packs: form, brief for Claude (copy or share to the Claude app), paste the answer, check, create or update the script. | Done |
| AN-7 | Settings: default pace, text size, focus line, countdown, answer limit, mirror. | Done |
| AN-8 | Listening, cue suggestions and smart follow. | On the PC only for now (needs whisper.cpp); planned for Android with on-device speech recognition. |

The parser, the brief and the import validation are Kotlin ports of the Go code. The Go test
`internal/golden` writes fixtures to `android/golden/`, and the Kotlin `GoldenTest` must produce
the same results, so a script, a brief or a sync hash means the same on both sides.

## What syncs
| Kind | Content exchanged | Conflicts |
|---|---|---|
| `script` | the Markdown file, verbatim | PC keeps its version under the id; the phone's becomes "<title> (phone copy)" on both sides |
| `pack` | `{"pack": pack.json, "answer": answer.md}` | same as scripts |
| `settings` (`prefs`) | pace, text size, mirror, focus line, countdown, animations, answer limit | the newer change wins |
| `rehearsals` | the run history JSON | merged (union by time, newest 20) |

Speech models, transcripts, listening settings and PC window settings stay on the PC.

## Sync rules (three-way)
For every item the phone compares its hash (L), the PC's (R) and the hash both had at the last
sync (B). L = R: nothing to do. Only the PC changed: pull (or delete on the phone). Only the phone
changed: push (or delete on the PC). Both changed: see the table. The PC refuses a push or delete
whose base is stale (HTTP 409) and the phone resolves it the same way. Deletions on the PC are
kept as tombstones so phones delete too. The phone syncs when it opens and a few seconds after
each change; "Sync now" is always there.

## Transport and security
| ID | Rule |
|---|---|
| SY-1 | The hub listens on TCP **47811** only while "Let paired phones sync with this PC" is on (Library → Phone sync), and advertises `_telecyborg._tcp` over mDNS (TXT `v`, `name`, `fp`). Windows may ask once to allow TeleCyborg on private networks. |
| SY-2 | HTTPS with TLS 1.3 and the PC's own ECDSA P-256 certificate (`%APPDATA%\TeleCyborg\sync\tls.pem`). |
| SY-3 | Pairing only while the PC user has "Pair a phone" open (3 minutes) and one phone at a time. |
| SY-4 | Numeric comparison with commitment (as in Macawlink): the PC commits to a nonce, the phone sends its nonce, the PC reveals; both show `SHA-256("telecyborg-pair-v1" ‖ certFP ‖ pcNonce ‖ phoneNonce)[0:4] mod 10⁶`. A man in the middle changes certFP on one side, so the codes differ. Both users must confirm. |
| SY-5 | The paired phone gets a random 256-bit token (the PC stores only its SHA-256) and pins the certificate fingerprint. Every sync request carries `X-TC-Phone` and `Authorization: Bearer`. |
| SY-6 | Forgetting a phone on the PC revokes it; the phone then shows "Pair again". |

## API (phones → PC hub)
| Method | Path | Purpose |
|---|---|---|
| GET | `/v1/hello` | app, version, PC name, short fingerprint |
| POST | `/v1/pair/begin` | `{phone_id, name}` → `{pair_id, commit, pc_name}` |
| POST | `/v1/pair/nonce` | `{pair_id, nonce}` → `{nonce}` (PC reveal) |
| POST | `/v1/pair/confirm` | `{pair_id, ok}` → waits for the PC user → `{ok, token}` |
| GET | `/v1/manifest` | every item: kind, id, hash, modified, deleted |
| GET | `/v1/item?kind=&id=` | one item with its content |
| PUT | `/v1/item` | `{kind, id, base, content}` → stored item, or 409 with the PC's current item |
| DELETE | `/v1/item?kind=&id=&base=` | delete, or 409 |

PC UI side: `/api/hub/status`, `/enable`, `/pair`, `/pair/cancel`, `/decide`, `/forget`.

## Tests
- `go test ./internal/hub`: pairing window, both-sides confirmation, conflicts, merges, tombstones.
- Kotlin `SyncTest`: the three-way rules against an in-memory PC, and **interop** against the real
  Go hub (`tools/hubtest`; set `TC_HUB_BIN`): pairing codes match, a wrong pin is refused, data
  moves both ways and a second sync reports "Up to date".
- `tools/e2e_phone_test.py`: pairing from the PC UI with a scripted phone.
