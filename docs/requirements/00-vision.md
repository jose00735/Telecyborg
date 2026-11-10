# 00 — Vision

## Problem
In interviews and customer meetings you want to deliver a tight, rehearsed narrative (a pitch,
a QBR story, a technical explanation) and still answer whatever comes up. Today that means
memorising, or reading from notes that don't keep pace, and losing the thread after a question.

## Product
TeleCyborg is a personal teleprompter and conversation co-pilot for one speaker on a video call
or on stage:

1. It shows the script at a readable size, scrolling at the pace you set or the pace you need to
   finish on time (stage 1).
2. It listens to the other side, spots keywords and suggests the prepared answer to jump to, then
   brings you back to where you left your narrative (stage 2).
3. It follows your own voice so the script moves with you, and coaches your pace from where you
   really are (stage 3).

## Users
- Primary: Jose — job interviews (pitches, "tell me about yourself", STAR stories) and customer
  meetings (QBRs, health-check readouts, demos) in English, Spanish and Portuguese.
- Secondary: anyone presenting on video who wants a private, offline prompter.

## Principles
- **Readable first.** The reading surface is calm and high-contrast; the Matrix styling lives in
  the frame, the transitions and the moments between screens, never in the text you read.
- **Offline and private.** One executable, no account, no cloud. Scripts are plain Markdown on
  disk. Audio (later stages) is processed locally and never stored unless you choose to.
- **Fast to drive.** Every reading action has a single key, and presentation clickers work.
- **Honest timing.** The app tells you the truth about pace: planned versus actual, per paragraph.

## Non-goals (for now)
- Multi-user or cloud sync.
- Video recording or streaming.
- Generating content inside the app: stage 2 exports a brief for Claude and imports the result.

## Success criteria for stage 1
- A 2-minute pitch can be pasted, timed to 2:00 and rehearsed in under a minute.
- The live display shows, per paragraph, how long it was planned to take and how long it took.
- When the speaker falls behind, the app shows the speed needed to finish on time and applies it
  with one key.
