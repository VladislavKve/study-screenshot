#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p dist
export CGO_ENABLED=1
VER="${VERSION:-0.2.0-cross}"
go test ./internal/...
go build -ldflags="-s -w -X main.version=$VER" -o dist/study-screenshot-linux-amd64 ./cmd/study-screenshot
if command -v x86_64-w64-mingw32-gcc >/dev/null; then
  CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ \
    GOOS=windows GOARCH=amd64 CGO_ENABLED=1 \
    go build -ldflags="-s -w -X main.version=$VER -H windowsgui" \
    -o dist/StudyScreenshot-windows-amd64.exe ./cmd/study-screenshot
fi
ls -lh dist/
