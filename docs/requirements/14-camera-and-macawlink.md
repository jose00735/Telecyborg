# 14 — Reading over your own camera image, and Macawlink (v1.1.0)

Goal: read the script right where your eyes already are on a video call — on your own camera
image — so you do not look away from the lens. Two private views; **the call never sees the
text**.

## 1. PC self-view (Camera, key V)
| ID | Requirement | Status |
|---|---|---|
| CV-1 | The prompter can show your camera image behind the text (mirrored like a self-view). | Done |
| CV-2 | It picks "Macawlink Camera" when present (the phone as webcam), otherwise the first camera. | Done |
| CV-3 | The reading line moves to 20 % from the top, right under a webcam on top of the screen; past paragraphs hide; a dark band keeps the text readable. | Done |
| CV-4 | Only this window shows the text; your call app receives the camera's clean picture (Windows shares the camera between apps). Combine with **H** (hide from screen sharing) and **T** (keep on top), and shrink the window under the lens. | Done |

## 2. On the phone screen, through Macawlink (Phone, key P)
Macawlink (github.com/jose00735/macawlink, v0.4.0+) turns the phone into the webcam. TeleCyborg
feeds the running prompter to Macawlink on the same PC; Macawlink forwards it over its USB/Wi-Fi
link; the Macawlink phone app shows it full screen next to the front lens. The text is drawn on
the phone's screen only — it never enters the encoded video.

```
TeleCyborg UI ──/api/macaw/*──▶ TeleCyborg server ──http://127.0.0.1:47801/api/prompter/*──▶ Macawlink PC
     ▲                                                                                       │ PROMPTER_SCRIPT 0x60
     └──────────── taps (toggle, next, prev, faster, slower) ◀── PROMPTER_KEY 0x63 ◀─ phone ◀─┘ PROMPTER_STATE 0x61 (8 Hz)
```

| ID | Requirement | Status |
|---|---|---|
| MW-1 | P turns the feed on; it stays on for later rehearsals until turned off. | Done |
| MW-2 | The phone gets the text of the current track (narrative or the open answer) and the position 8 times a second, with a speed so it moves smoothly between updates. | Done |
| MW-3 | The phone shows the text beside the lens (left/right edge in landscape, top in portrait), mirror and size from TeleCyborg, the clock and pace alerts. | Done |
| MW-4 | Tap on the phone = play/pause; swipe up/down = next/previous paragraph; TeleCyborg reacts. | Done |
| MW-5 | Stopping the run clears the phone. A phone that reconnects gets the current text again. | Done |
| MW-6 | Only phones that announce `"prompter"` in HELLO get prompter messages (older Macawlink phones would drop the link on unknown types). | Done |

Message formats: Macawlink `docs/PROTOCOL.md` and `proto-golden/prompter.json`.

## Tests
- Macawlink `go test ./internal/app ./internal/web`: feed to capable phones only, re-sent on
  reconnect, taps back, clear; API guards.
- Macawlink Kotlin `PrompterTest` + a rendered landscape snapshot of the phone view.
- `tools/e2e_phone_test.py`: TeleCyborg UI → TeleCyborg server → real Macawlink receiver logic
  (`macawlink/windows/cmd/devreceiver`) → fake phone, and a phone tap back to the PC prompter;
  camera self-view with Chrome's fake camera.
