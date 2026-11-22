# 10 — Privacy and consent

Stage 1 never touches audio. From stage 2 on, TeleCyborg listens to conversations, which includes
other people's voices. These rules are part of the product, not an afterthought.

| ID | Rule | How v1.0.0 does it |
|---|---|---|
| PV-1 | All audio processing happens on the computer. No audio or transcript leaves the machine. | whisper-server runs on 127.0.0.1; the only network use is the one-time download of the runtime and model, which the user starts. |
| PV-2 | Listening is off by default and needs an explicit toggle each session; a visible indicator shows when it is on. | Every rehearsal starts silent; L / Listen opens the dialog; a pulsing red "● LISTENING" badge and a red Listen button show capture. |
| PV-3 | Audio is never written to disk. Transcripts are kept in memory and discarded at the end of the session unless "save transcript" is chosen. | Chunks live only in memory requests; "Save transcript and notes for this session" is unticked every time. |
| PV-4 | The first time listening is enabled, TeleCyborg explains that recording or transcribing other people may require their consent (for example under Colombia's Law 1581 of 2012, US two-party-consent states and the GDPR) and that the user is responsible for obtaining it. | The dialog shows the notice and needs "I understand" before the first start. |
| PV-5 | Customer meetings: follow the customer's and your employer's policies on recording and AI tools. | Stated in the notice. |
| PV-6 | No telemetry, analytics or update checks without opt-in. | None. |
| PV-7 | TeleCyborg can hide its window from screen sharing and recordings. | H / "Hide in share" (Windows 10 2004+). |
