# 11 — Build and release

| ID | Requirement |
|---|---|
| BR-1 | Target: `windows/amd64`, `CGO_ENABLED=0`, `-trimpath -ldflags "-s -w -H windowsgui"`. |
| BR-2 | Version injected with `-X main.version=vX.Y.Z`; shown in the library footer and `/api/info`. |
| BR-3 | `go vet ./...` and `go test ./...` must pass before a release (build.sh runs them). |
| BR-4 | Releases are Git tags `vX.Y.Z` with zips attached (browsers block raw `.exe` downloads). |
| BR-5 | `windows/arm64` is built as a portable zip (the speech engine runs under x64 emulation). |
| BR-6 | Code signing is planned; until then the README explains "More info → Run anyway". |
| BR-7 | WebView2 is not bundled; if it is missing the app opens in the default browser and says so. |
| BR-8 | Windows builds are delivered as an **installer inside a zip**: per-user install to `%LOCALAPPDATA%\Programs\TeleCyborg`, Start menu and desktop shortcuts, Settings → Apps entry, uninstaller (keeps the user's scripts unless asked), `/S` silent install. |
| BR-9 | The installer bundles the whisper.cpp runtime (MIT) and THIRD_PARTY_NOTICES.txt, and offers to install the Microsoft Visual C++ runtime when it is missing. |
| BR-10 | The exe carries an icon and a manifest (per-monitor DPI aware); the installer's manifest is `asInvoker`. |

```bash
./build.sh v1.0.0      # → dist/
```

| File | Contents |
|---|---|
| `TeleCyborg-<v>-windows-amd64-setup.zip` | `TeleCyborg-Setup-<v>.exe` + README |
| `TeleCyborg-<v>-windows-amd64-portable.zip` | `telecyborg.exe`, `whisper\` runtime, README, notices |
| `TeleCyborg-<v>-windows-arm64.zip` | portable exe for ARM PCs |
| `TeleCyborg-<v>-android.apk` | Android app (built when `ANDROID_HOME` is set) |
| `SHA256SUMS.txt` | checksums |

Icons and manifests are compiled into `rsrc_windows_*.syso` with
`rsrc -manifest assets/telecyborg.exe.manifest -ico assets/icon.ico` (and the installer's in
`cmd/installer`).

| BR-11 | The Android APK is built with `./gradlew assembleRelease` (JDK 17+, Android SDK 35) and signed with the debug key unless `TELECYBORG_KEYSTORE` / `_PASSWORD` / `_ALIAS` are set. Sideload it (allow "install unknown apps"). |

## Tests
- `go test ./...` — parser, matching, alignment, packs, speech helpers, API (with a fake whisper server).
- `python3 tools/e2e_test.py` — Chrome with a fake microphone and `tools/fake_whisper.py`: pack →
  script → listening → suggestion → detour and return → search → follow → report.
- `cd android && ./gradlew testDebugUnitTest` — Kotlin parser/brief/hash against the Go fixtures,
  sync rules, and interop with the real hub when `TC_HUB_BIN` points to `go build ./tools/hubtest`.
- `./gradlew recordPaparazziDebug` — renders the Android screens on the JVM (no emulator).
- `python3 tools/e2e_phone_test.py telecyborg <macawlink devreceiver>` — sync pairing UI, camera view, Macawlink feed.
