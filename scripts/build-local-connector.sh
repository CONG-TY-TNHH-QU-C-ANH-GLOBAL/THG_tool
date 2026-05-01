#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-${VERSION:-dev}}"
OUT_DIR="${OUT_DIR:-data/downloads}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR="$ROOT/$OUT_DIR"

mkdir -p "$OUTPUT_DIR"

if [ -z "${GOCACHE:-}" ]; then
  export GOCACHE="$ROOT/.gocache"
fi

build_target() {
  local goos="$1"
  local goarch="$2"
  local name="$3"
  echo "Building $name for $goos/$goarch"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -buildvcs=false -ldflags="-s -w -X main.version=$VERSION" \
    -o "$OUTPUT_DIR/$name" ./cmd/thg-login
}

build_target windows amd64 thg-login-windows.exe
build_target linux amd64 thg-login-linux
build_target darwin amd64 thg-login-mac-intel
build_target darwin arm64 thg-login-mac-m1

chmod 0644 "$OUTPUT_DIR"/thg-login-*
if [ -d "$ROOT/local-connector-extension" ]; then
  echo "Packaging thg-chrome-extension.zip"
  rm -f "$OUTPUT_DIR/thg-chrome-extension.zip"
  if command -v python3 >/dev/null 2>&1; then
    (cd "$ROOT/local-connector-extension" && python3 -m zipfile -c "$OUTPUT_DIR/thg-chrome-extension.zip" .)
  else
    (cd "$ROOT/local-connector-extension" && zip -qr "$OUTPUT_DIR/thg-chrome-extension.zip" .)
  fi
  chmod 0644 "$OUTPUT_DIR/thg-chrome-extension.zip"
fi
echo "Local Connector builds written to $OUTPUT_DIR"
