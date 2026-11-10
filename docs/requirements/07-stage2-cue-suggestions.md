# 07 — Stage 2: Cue suggestions (planned, v0.3–v0.4)

Goal: while you present, TeleCyborg listens to the **other side** of the call, spots what they are
asking about, and offers the prepared section that answers it. You jump there, answer, and return
to where you left the narrative.

## Requirements
| ID | Requirement | P |
|---|---|---|
| CS-1 | Capture system audio output (the other people on the call) separately from your microphone. | M |
| CS-2 | Transcribe locally, near real time (< 2 s delay), in English, Spanish and Portuguese. | M |
| CS-3 | Match the transcript against each section's `keywords:` and title (stemmed, accent-insensitive, synonyms). | M |
| CS-4 | Show up to three suggestion chips with the section name and the words that matched; newest first; fade after 20 s. | M |
| CS-5 | Jump to a suggestion with a number key (1–3) or click. | M |
| CS-6 | "Back to narrative" (Backspace) returns to the paragraph you left; a breadcrumb shows the jump stack. | M |
| CS-7 | Q&A sections (from content packs) are hidden from the main flow and only reached through suggestions or search. | M |
| CS-8 | Quick search (Ctrl+K) across sections when no suggestion fits. | S |
| CS-9 | A live transcript strip, collapsible, for context. | S |
| CS-10 | Nothing is recorded to disk unless "save transcript" is turned on per session. | M |

## Speech-to-text options (decision in v0.4)
| Option | Pros | Cons |
|---|---|---|
| whisper.cpp as a bundled helper exe | Accurate, multilingual, offline | Larger download (model 75–500 MB); CPU load |
| Vosk | Small models, streaming, offline | Lower accuracy for Spanish/Portuguese |
| Windows speech recognition (WinRT) | Built in | Weaker accuracy; language packs needed |
| Cloud API | Best accuracy | Not offline; consent and cost concerns |

Proposal: whisper.cpp helper with the `base` multilingual model, streamed in 2–3 s windows;
system audio captured with WASAPI loopback.

## Matching
- Each section has `keywords:` (from the content pack) plus automatic keywords from its title.
- Score = matched keyword weight × recency; ties broken by section order.
- Questions phrased as "tell me about…", "how do you…", "what would you…" boost Q&A sections.
