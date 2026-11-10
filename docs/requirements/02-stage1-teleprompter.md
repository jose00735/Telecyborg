# 02 — Stage 1: Teleprompter (v0.1.0)

Priority: **M** = must (in v0.1.0), **S** = should (v0.1.x if not in first beta), **C** = could.

## Library
| ID | Requirement | P |
|---|---|---|
| LIB-1 | List saved scripts with title, word count, estimated duration at their pace, last edited. | M |
| LIB-2 | Create a new script. | M |
| LIB-3 | Import a `.md` or `.txt` file by file picker or drag and drop. | M |
| LIB-4 | Open a script in the editor or straight into rehearsal. | M |
| LIB-5 | Delete a script with an in-page confirmation (no browser dialogs). | M |
| LIB-6 | Empty state explains how to add the first script and offers a sample. | M |

## Editor
| ID | Requirement | P |
|---|---|---|
| ED-1 | Edit title and body (Markdown, see 04-script-format.md). | M |
| ED-2 | Set pace as words per minute (60–300, default 140). | M |
| ED-3 | Set an optional target duration (`m:ss`); "Fit to time" computes the wpm that meets it. | M |
| ED-4 | Live stats: words, paragraphs, sections, estimated duration at current pace. | M |
| ED-5 | Save with Ctrl+S; unsaved changes are flagged. | M |
| ED-6 | Paste rich text strips formatting. | S |

## Prompter
| ID | Requirement | P |
|---|---|---|
| PR-1 | Large, high-contrast text with a focus line at ~38% of the window height. | M |
| PR-2 | 3-second countdown before scrolling starts. | M |
| PR-3 | Smooth auto-scroll so each paragraph crosses the focus line in its planned time. | M |
| PR-4 | Play/pause (Space), faster/slower ±5 wpm (↑/↓), next/previous paragraph (→/←, Page Down/Up), restart (Home). | M |
| PR-5 | Presentation clickers (they send Page Up/Down or arrows) control paragraphs. | M |
| PR-6 | Text size + / − and remembered per script. | M |
| PR-7 | Mirror mode (M) for beam-splitter glass. | M |
| PR-8 | Keep window on top (T) — native window only. | M |
| PR-9 | Clicking a paragraph jumps to it; the mouse wheel nudges the position. | M |
| PR-10 | Section headings are shown as quiet markers, not read aloud. | M |
| PR-11 | `[pause]` markers hold the scroll for the given time. | M |
| PR-12 | Esc returns to the editor (or library) and keeps the run's report. | M |

## Timing and coaching (details in 03-pacing-and-timing.md)
| ID | Requirement | P |
|---|---|---|
| TM-1 | HUD shows: state, current wpm, elapsed / target, paragraph n of N, paragraph time actual / planned. | M |
| TM-2 | Timeline bar with one segment per paragraph, sized by planned time, coloured when over or under. | M |
| TM-3 | Pace alert when behind the plan by more than 3 s: shows the deficit and the wpm needed to finish on target; A applies it. | M |
| TM-4 | Ahead notice when more than 5 s ahead. | S |
| TM-5 | End-of-run report: total, per paragraph planned vs actual, slowest paragraphs. | M |
| TM-6 | Rehearsal history saved per script (last 20 runs). | M |

## Settings and window
| ID | Requirement | P |
|---|---|---|
| WIN-1 | Native window via WebView2 on Windows; falls back to the default browser if WebView2 is missing or `-browser` is passed. | M |
| WIN-2 | Remembers default wpm, text size, mirror and focus-line position. | M |
| WIN-3 | Single instance per user is not required in v0.1.0. | C |

## Acceptance tests
1. Paste the Terrific 90-second pitch, set target 1:30, Fit to time → wpm shown ≈ words/1.5.
2. Rehearse: total planned time in the HUD equals 1:30 ± 1 s.
3. Pause for 10 s mid-run → amber alert shows "behind ~0:10" and a higher wpm; pressing A applies it
   and the projected finish returns to 1:30 ± 2 s.
4. Press → three times → the current paragraph advances by three; the timeline marks skipped ones.
5. Finish → report lists every paragraph with planned and actual times; history shows the run.
