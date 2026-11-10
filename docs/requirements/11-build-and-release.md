# 11 — Build and release

| ID | Requirement |
|---|---|
| BR-1 | Target for every beta: `windows/amd64`, single exe, `CGO_ENABLED=0`, `-trimpath -ldflags "-s -w -H windowsgui"`. |
| BR-2 | Version injected with `-X main.version=vX.Y.Z`; shown in the library footer and `/api/info`. |
| BR-3 | `go vet ./...` and `go test ./...` must pass before a release. |
| BR-4 | Releases are Git tags `vX.Y.Z` with the exe attached (zipped, since some browsers block raw `.exe` downloads). |
| BR-5 | `windows/arm64` is built when it costs nothing extra (same command, `GOARCH=arm64`). |
| BR-6 | Code signing is planned for v1.0 to avoid SmartScreen warnings; until then the README explains "More info → Run anyway". |
| BR-7 | WebView2 is not bundled; if it is missing the app opens in the default browser and says so. |

```bash
VERSION=v0.1.0
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "-s -w -H windowsgui -X main.version=$VERSION" -o telecyborg.exe .
```
