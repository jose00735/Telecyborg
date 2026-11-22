#!/usr/bin/env bash
# Builds the Windows release of TeleCyborg into dist/:
#   TeleCyborg-<v>-windows-amd64-setup.zip     installer (TeleCyborg + offline speech engine)
#   TeleCyborg-<v>-windows-amd64-portable.zip  no install: telecyborg.exe + whisper\ folder
#   TeleCyborg-<v>-windows-arm64.zip            portable exe for ARM PCs (speech runs under x64 emulation)
# Usage: ./build.sh v1.0.0
set -euo pipefail
VERSION="${1:-v1.0.0-dev}"
WHISPER="v1.8.4"
cd "$(dirname "$0")"
DIST=dist; CACHE=.cache; STAGE="$DIST/stage"
rm -rf "$DIST"; mkdir -p "$DIST" "$CACHE" "$STAGE/whisper"

echo "== checks"
go vet ./... && GOOS=windows go vet ./...
go test ./...

echo "== whisper.cpp $WHISPER runtime"
ZIP="$CACHE/whisper-bin-x64-$WHISPER.zip"
[ -s "$ZIP" ] || curl -fsSL -o "$ZIP" "https://github.com/ggml-org/whisper.cpp/releases/download/$WHISPER/whisper-bin-x64.zip"
[ -s "$CACHE/whisper-LICENSE-$WHISPER" ] || curl -fsSL -o "$CACHE/whisper-LICENSE-$WHISPER" "https://raw.githubusercontent.com/ggml-org/whisper.cpp/$WHISPER/LICENSE"
for f in whisper-server.exe whisper.dll ggml.dll ggml-base.dll ggml-cpu.dll; do
  unzip -p "$ZIP" "Release/$f" > "$STAGE/whisper/$f"
done
cp "$CACHE/whisper-LICENSE-$WHISPER" "$STAGE/whisper/LICENSE-whisper.cpp.txt"

LDFLAGS="-s -w -H windowsgui -X main.version=$VERSION"
echo "== telecyborg.exe (amd64, arm64)"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$LDFLAGS" -o "$STAGE/telecyborg.exe" .
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "$LDFLAGS" -o "$DIST/telecyborg-arm64.exe" .
cp README-windows.txt "$STAGE/README.txt"
cp THIRD_PARTY_NOTICES.txt "$STAGE/THIRD_PARTY_NOTICES.txt"

echo "== installer"
rm -f cmd/installer/payload/payload.zip
(cd "$STAGE" && zip -q -r -X "../../cmd/installer/payload/payload.zip" telecyborg.exe whisper README.txt THIRD_PARTY_NOTICES.txt)
SETUP="TeleCyborg-Setup-$VERSION.exe"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -H windowsgui -X main.version=$VERSION" -o "$DIST/$SETUP" ./cmd/installer
rm -f cmd/installer/payload/payload.zip

echo "== zips"
(cd "$DIST" && cp ../README-windows.txt README.txt && zip -q -X "TeleCyborg-$VERSION-windows-amd64-setup.zip" "$SETUP" README.txt && rm README.txt)
(cd "$STAGE" && zip -q -r -X "../TeleCyborg-$VERSION-windows-amd64-portable.zip" telecyborg.exe whisper README.txt THIRD_PARTY_NOTICES.txt)
(cd "$DIST" && cp telecyborg-arm64.exe telecyborg.exe && cp ../README-windows.txt README.txt && zip -q -X "TeleCyborg-$VERSION-windows-arm64.zip" telecyborg.exe README.txt && rm telecyborg.exe README.txt telecyborg-arm64.exe)
rm -rf "$STAGE"

if [ -n "${ANDROID_HOME:-}" ]; then
  echo "== Android APK"
  (cd android && ./gradlew --no-daemon -q testDebugUnitTest assembleRelease)
  cp android/app/build/outputs/apk/release/app-release.apk "$DIST/TeleCyborg-$VERSION-android.apk"
fi
(cd "$DIST" && sha256sum *.zip *.apk 2>/dev/null > SHA256SUMS.txt)
ls -la "$DIST"
