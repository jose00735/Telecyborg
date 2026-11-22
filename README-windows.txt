TeleCyborg — teleprompter that listens and follows your voice
=============================================================

INSTALL
  1. Unzip and run TeleCyborg-Setup-<version>.exe.
  2. Windows SmartScreen may say "Windows protected your PC" because the app is not code-signed
     yet: click "More info" → "Run anyway".
  3. Setup installs for your user only (no admin rights) into
     %LOCALAPPDATA%\Programs\TeleCyborg, adds Start menu and desktop shortcuts, and an entry in
     Settings → Apps (use it to uninstall). Your scripts live in %APPDATA%\TeleCyborg.

FIRST LISTENING SESSION
  - In the prompter press L (or "Listen"). The first time, TeleCyborg downloads a speech model
    once (Base, 57 MB); after that everything runs offline on this PC. No audio is ever saved.
  - If setup said the Microsoft Visual C++ runtime was missing and you skipped it, install
    vc_redist.x64.exe from https://aka.ms/vs/17/release/vc_redist.x64.exe before listening.
  - The other side of a call: choose "System audio", then share your ENTIRE screen and tick
    "Share system audio". TeleCyborg ignores the picture.
  - Recording or transcribing other people may need their consent. That is your responsibility.

KEYS IN THE PROMPTER
  Space play/pause · ↑/↓ pace · →/← paragraph · A apply suggested pace · L listen
  F scroll mode (clock / follow my voice / hybrid) · 1-3 jump to a suggestion
  Backspace back to the narrative · Ctrl+K find a section · C live transcript
  H hide from screen sharing · T keep on top · M mirror · +/− text size · Esc stop

Requirements: Windows 10 or 11 (x64; ARM64 has a separate portable build), Microsoft Edge
WebView2 runtime (built into Windows 10/11). Source: https://github.com/jose00735/telecyborg
