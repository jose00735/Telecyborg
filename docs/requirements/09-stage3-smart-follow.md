# 09 — Stage 3: Smart follow (planned, v0.5)

Goal: the script moves with your voice. You speak, TeleCyborg knows exactly which word you are
on, scrolls to keep it on the focus line, and coaches your pace from your real position.

## Requirements
| ID | Requirement | P |
|---|---|---|
| SM-1 | Transcribe your microphone locally in near real time. | M |
| SM-2 | Align the transcript to the script with fuzzy word matching that tolerates skipped, repeated and paraphrased words. | M |
| SM-3 | Drive the script clock `s` (see 03) from the aligned position; smooth it so scrolling never jumps backwards. | M |
| SM-4 | When you go off script for more than 8 s, hold the position and show "off script"; resume when you return. | M |
| SM-5 | Pace coaching uses your measured speaking wpm over the last 20 s. | M |
| SM-6 | Rehearsal report adds: actual wpm per paragraph, filler words (um, eh, este, like), long silences. | S |
| SM-7 | Hybrid mode: clock-driven scrolling that self-corrects to the voice position. | S |
| SM-8 | Works in English, Spanish and Portuguese; language from the script's `lang`. | M |

## Approach
- Streaming ASR with partial results (same engine as stage 2, microphone input).
- Alignment: rolling window over the next ~60 script words; score each candidate position with
  normalised edit distance on the last 6–10 spoken words; accept when confidence > threshold.
