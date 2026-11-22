# 08 — Content packs (implemented in v1.0.0)

A content pack is everything for one occasion — an interview, a QBR, a demo — in one folder:

```
%APPDATA%\TeleCyborg\packs\terrific-interview\
├── pack.json        the pack form
├── pack.md          the same brief, readable
├── claude-brief.md  the prompt to give Claude
└── answer.md        Claude's last answer, as pasted
```

The imported pitch and Q&A become **one script** in the library (`scripts\<id>.md`) with
`pack: <id>` in its front matter; the Q&A part follows `=== QA ===` (see 04). Importing again
updates the same script.

## Requirements
| ID | Requirement | P | Status |
|---|---|---|---|
| CP-1 | Pack form: occasion, audience, goal, pitch length, pace, language, topics, must-mention facts, things to avoid, sources, number of questions, answer length. | M | Done. |
| CP-2 | "Save & write brief" writes `claude-brief.md`: role, audience, goal, timing (words in total and per section from the target time and pace), facts, sources, keyword instructions and the exact output format. | M | Done. |
| CP-3 | One-click copy of the brief; open claude.ai in the browser. | M | Done. |
| CP-4 | Import Claude's answer by paste; validate the format and show what will be created. | M | Done: sections, questions, words, duration against target, `[check]` count; errors block, warnings inform (missing keywords, long answers, over target). Pasting checks automatically. |
| CP-5 | Pitch and Q&A become a script linked to the pack; Q&A sections become cue targets. | M | Done. |
| CP-6 | Packs can be exported as a zip and imported on another machine. | S | Done: export writes `<id>.telecyborg-pack.zip` to Downloads, including the script. |

## Output format requested from Claude
```markdown
---
title: <pitch title>
target: <m:ss>
wpm: <pace>
lang: <en|es|pt>
---
# <Section title>
keywords: <comma-separated words a listener would say that should trigger this section>
<paragraphs>

=== QA ===
# Q: <likely question>
keywords: <...>
<answer paragraphs, within the answer length>
```

TeleCyborg takes the first fenced code block of the reply (or the whole reply when there is none).

## Facts discipline
The brief tells Claude to use only the facts and sources provided, never to invent numbers,
names, dates, customers or results, and to mark anything it cannot support with `[check]`, which
TeleCyborg counts in the import preview and the editor ("next" selects each one) and highlights in
the prompter.
