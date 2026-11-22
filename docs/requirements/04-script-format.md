# 04 — Script format

Scripts are UTF-8 Markdown files with an optional front-matter block.

```markdown
---
title: Terrific — 90 second pitch
wpm: 140
target: 1:30
fontSize: 44
---

# Opening
Hi, I'm Jose Acosta. I'm a Customer Success Manager based in Colombia...

[pause 1.5]

# Scale
At Netskope I managed a book of more than 2,000 SaaS accounts...
```

## Rules
| ID | Rule |
|---|---|
| SF-1 | Front matter is optional; keys: `title`, `wpm`, `target` (`m:ss` or seconds), `fontSize`, `lang`. Unknown keys are kept. |
| SF-2 | `#`, `##`, `###` lines start a **section**; the heading is shown as a marker and not counted as words. |
| SF-3 | Paragraphs are separated by one or more blank lines. Single line breaks inside a paragraph are kept as line breaks. |
| SF-4 | `[pause]` on its own line adds a 1.5 s pause after the previous paragraph; `[pause 3]` or `[pause 3s]` sets the length. |
| SF-5 | `**bold**` renders as emphasis (stress this); `_text_` renders as a softer aside. Other Markdown is shown as plain text. |
| SF-6 | `<!-- notes -->` are hidden from the prompter (speaker notes, sources). |
| SF-7 | Lines starting with `>` are cues for the speaker (e.g. `> smile`, `> show slide 3`): shown smaller, not counted as words. |
| SF-8 | Words are counted as whitespace-separated tokens containing a letter or digit. |
| SF-9 | `keywords: a, b, c` under a heading lists words the listener would say that should bring the section up. Commas or semicolons separate keywords; `price/cost` gives alternatives of one idea. Not shown in the prompter. |
| SF-10 | A line `=== QA ===` starts the Q&A part: every section after it is an answer. A heading that starts with `Q:` is an answer even without the marker. Answers are not in the main flow, timeline or target time; they are reached through suggestions, search (Ctrl+K) or jumps. |
| SF-11 | `[check]` marks a fact to verify (Claude adds it when unsure). It is highlighted in the prompter, counted in the editor and not counted as a word. |
| SF-12 | Front matter `pack: <id>` links a script to its content pack; `lang: en|es|pt` sets the speech language for smart follow. |

## Example with Q&A
```markdown
# Netskope results
keywords: netskope, adoption/utilisation, rollout
At Netskope I moved an account from below 50 to 70 percent utilisation.

=== QA ===
# Q: What is your biggest weakness?
keywords: weakness, improve
I over-prepare, so now I time-box it.
```

## Storage
- One file per script: `%APPDATA%\TeleCyborg\scripts\<slug>.md`.
- Rehearsals: `%APPDATA%\TeleCyborg\rehearsals\<slug>.json` (last 20 runs).
- Settings: `%APPDATA%\TeleCyborg\settings.json`.
- Content packs: `%APPDATA%\TeleCyborg\packs\<id>\` (see 08).
- Speech models: `%APPDATA%\TeleCyborg\speech\models\`.
- Transcripts and notes, only when "save transcript" is on: `%APPDATA%\TeleCyborg\transcripts\`.
- On Linux/macOS the base folder is the OS user-config directory.
