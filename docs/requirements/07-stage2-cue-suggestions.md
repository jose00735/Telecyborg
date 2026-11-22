# 07 — Stage 2: Cue suggestions (implemented in v1.0.0)

Goal: while you present, TeleCyborg listens to the **other side** of the call, spots what they are
asking about, and offers the prepared section that answers it. You jump there, answer, and return
to where you left the narrative.

## Requirements
| ID | Requirement | P | Status |
|---|---|---|---|
| CS-1 | Capture system audio output (the other people on the call) separately from your microphone. | M | Done: "System audio" uses screen-share audio (share the entire screen, tick "Share system audio"); "An input device" takes Stereo Mix or a virtual cable. Your microphone is a separate capture. |
| CS-2 | Transcribe locally, near real time (< 2 s delay), in English, Spanish and Portuguese. | M | Done with whisper.cpp; delay is the end of the phrase plus 0.3–1 s on a modern CPU with the Base model. Language per side, or detect. |
| CS-3 | Match the transcript against each section's `keywords:` and title (stemmed, accent-insensitive, synonyms). | M | Done (`internal/cue`). |
| CS-4 | Show up to three suggestion chips with the section name and the words that matched; newest first; fade after 20 s. | M | Done. |
| CS-5 | Jump to a suggestion with a number key (1–3) or click. | M | Done. |
| CS-6 | "Back to narrative" (Backspace) returns to the paragraph you left; a breadcrumb shows the jump stack. | M | Done. |
| CS-7 | Q&A sections (from content packs) are hidden from the main flow and only reached through suggestions or search. | M | Done: answers open as a detour with their own clock. |
| CS-8 | Quick search (Ctrl+K) across sections when no suggestion fits. | S | Done; also "suggest from what they asked" for typed questions. |
| CS-9 | A live transcript strip, collapsible, for context. | S | Done (C). |
| CS-10 | Nothing is recorded to disk unless "save transcript" is turned on per session. | M | Done. |
| CS-11 | Answer-length guard: the answer bar warns at the limit (default 60 s) and at 1.5×. | S | Done. |
| CS-12 | Post-meeting notes: questions you were asked, sections you used, filler words; copy or save. | S | Done. |

## Speech-to-text decision
| Option | Pros | Cons |
|---|---|---|
| **whisper.cpp `whisper-server` (chosen)** | Accurate, multilingual, offline; official Windows build; model stays loaded | Model download (31–181 MB); CPU load while listening; needs the Visual C++ runtime |
| Vosk | Small models, streaming | Lower accuracy for Spanish/Portuguese |
| Windows speech recognition (WinRT) | Built in | Weaker accuracy; language packs; hard from pure Go |
| Cloud API | Best accuracy | Not offline; breaks PV-1 |

Models offered: Base q5_1 (57 MB, default), Tiny q5_1 (31 MB), Small q5_1 (181 MB), downloaded
from the whisper.cpp project's Hugging Face repository; Base is checked against its SHA-256. A
model file can also be imported from disk. The installer bundles the whisper.cpp runtime; the
portable exe downloads it from the whisper.cpp GitHub release on first use.

Each request carries a prompt: the cue keywords for the other side, and the upcoming script words
for the speaker, so names and jargon come out spelled like the script.

## Matching
- Each section has `keywords:` plus automatic keywords from its title (content words, and the
  whole title as a phrase). Explicit keywords weigh 1.0 (1.4 for phrases), title words 0.5.
- Words are folded (case, accents, punctuation) and lightly stemmed for EN/ES/PT; words one
  letter apart match ("Netscope" ~ "Netskope"). Multi-word keywords allow one extra word between.
- `a/b` in a keyword gives alternatives; built-in synonym groups cover common interview and
  customer words across the three languages (salary/salario/remuneração, weakness/debilidad…).
- A phrase used by several sections weighs less (÷ √sections).
- Score = Σ weight × 0.5^(age / 10 s) over the last 25 s; a suggestion needs score ≥ 1.0 and a
  match in the newest chunk (so a decaying match is not suggested again).
- Question phrasing ("tell me about…", "¿cómo…", "o que…", "?") boosts Q&A sections by 30 %.
