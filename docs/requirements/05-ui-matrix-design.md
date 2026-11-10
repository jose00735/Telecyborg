# 05 — UI: the Matrix design

The brief: it should look like *The Matrix*, with fancy Matrix animations between screens.
The rule that keeps it usable: **the frame is the Matrix, the text you read is not.**

## Tokens
| Token | Value | Use |
|---|---|---|
| `--void` | `#000500` | Background (black with a green bias, like an unlit CRT) |
| `--code` | `#00ff41` | Phosphor green: rain, active controls, focus line |
| `--trail` | `#0b6e2a` | Dimmed green: borders, inactive UI, rain tails |
| `--read` | `#d8ffe0` | Reading text: pale mint, softer than pure green for long reading |
| `--ghost` | `#5f8f69` | Secondary text, past paragraphs |
| `--warn` | `#ffb000` | Pace alert only (semantic, never decoration) |
| `--miss` | `#ff4d4d` | Can't-finish state and destructive actions |

Typography: one family — the Windows monospace stack `"Cascadia Mono", "Cascadia Code",
Consolas, "Lucida Console", monospace` (offline, no web fonts). Reading text 32–72 px with
1.45 line height and at most ~42 characters per line; UI text 14–16 px.

## Screens
| Screen | Purpose | Layout |
|---|---|---|
| Boot | First launch only: the name decrypts out of the rain, then the library fades in. | full-bleed rain |
| Library | Pick or add a script. | list of scripts as terminal rows: title, words, duration, edited |
| Editor | Write or paste, set pace or target time. | text area left, pace panel right (stacks on narrow windows) |
| Prompter | Read. | text column centred, focus band at 38%, HUD bottom, timeline under it |
| Report | See how the run went. | per-paragraph rows with planned vs actual bars |

## Motion
| ID | Moment | Behaviour |
|---|---|---|
| MO-1 | Screen change | Digital-rain wipe: columns of katakana and digits fall over the old screen (~350 ms), the new screen's headline decrypts from random glyphs (~400 ms). |
| MO-2 | Countdown | 3-2-1 rendered as large glyph-scrambled digits. |
| MO-3 | Behind alert | The amber bar slides up once; no blinking. |
| MO-4 | Idle | Faint rain behind the library at 8% opacity; none behind the prompter text. |
| MO-5 | Reduced motion | `prefers-reduced-motion` replaces wipes with a 120 ms fade and disables rain. |

## Readability rules
- No rain, glow or scanlines behind the reading column.
- Past paragraphs dim to `--ghost`; the current one is `--read` at full strength; upcoming ones at 80%.
- Focus band: two thin `--code` hairlines with a faint fill; never covers text.
- All controls reachable by keyboard with a visible focus ring.
