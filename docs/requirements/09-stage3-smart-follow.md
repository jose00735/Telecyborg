# 09 — Stage 3: Smart follow (implemented in v1.0.0)

Goal: the script moves with your voice. You speak, TeleCyborg knows which word you are on,
scrolls to keep it on the focus line, and coaches your pace from your real position.

## Requirements
| ID | Requirement | P | Status |
|---|---|---|---|
| SM-1 | Transcribe your microphone locally in near real time. | M | Done: 2.4 s windows with 0.6 s overlap, sent as soon as you pause or the window fills. |
| SM-2 | Align the transcript to the script with fuzzy word matching that tolerates skipped, repeated and paraphrased words. | M | Done (`internal/follow`). |
| SM-3 | Drive the script clock `s` from the aligned position; smooth it so scrolling never jumps backwards. | M | Done. |
| SM-4 | When you go off script for more than 8 s, hold the position and show "off script"; resume when you return. | M | Done. |
| SM-5 | Pace coaching uses your measured speaking wpm over the last 20 s. | M | Done ("Spoken" in the HUD; the behind alert tells you the pace to speak at). |
| SM-6 | Rehearsal report adds: actual wpm per paragraph, filler words, long silences. | S | Done, plus off-script time. |
| SM-7 | Hybrid mode: clock-driven scrolling that self-corrects to the voice position. | S | Done: the clock runs at the set pace, waits when it is 2.5 s ahead of your voice and catches up when you are ahead. |
| SM-8 | Works in English, Spanish and Portuguese; language from the script's `lang`. | M | Done. |

Modes (F cycles them): **Clock** (stage 1), **Follow voice**, **Hybrid**. Follow and hybrid need
"Me" turned on in the listening dialog. Manual moves (arrows, click, wheel, jumps) re-seed the
aligner so the voice does not pull the script back.

## Approach
- Script tokens line up one to one with the editor's word counts, so a position (paragraph, word)
  maps straight onto the prompter's plan clock: `s = start + (word / words) × read`.
- For each chunk: drop hesitations, drop the start of the chunk when it repeats the end of the
  previous one (overlapping windows), keep the last 10 spoken words.
- Candidates: every script word from 8 behind to 60 ahead of the current position. Score = the
  longest common subsequence between the spoken words and the script window ending at the
  candidate (fuzzy word equality), minus half a point for every spoken word after the match,
  divided by the number of spoken words. Accept at ≥ 0.5 with at least 3 matched words; a jump of
  more than 30 words needs ≥ 0.7 and 5 words. If nothing matches nearby, the rest of the track is
  searched strictly (≥ 0.75, 6 words) to catch skipped paragraphs.
- The position only moves forward. The UI eases toward it, keeps the line slightly ahead while you
  speak, and lets a `[pause]` play by the clock once you finish its paragraph.
- Filler words are counted from the speaker's transcript; whisper is prompted with a hesitant
  lead-in ("Um, so, uh…", "Eh, bueno, este…", "Hum, então, né…") so it keeps them.
- Silences over 3 s while the run plays are counted from the speech detector.
