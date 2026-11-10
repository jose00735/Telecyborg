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
| SF-9 | Stage 2 adds `keywords:` per section (see 08-content-packs.md); stage 1 ignores them. |

## Storage
- One file per script: `%APPDATA%\TeleCyborg\scripts\<slug>.md`.
- Rehearsals: `%APPDATA%\TeleCyborg\rehearsals\<slug>.json` (last 20 runs).
- Settings: `%APPDATA%\TeleCyborg\settings.json`.
- On Linux/macOS the base folder is the OS user-config directory.
