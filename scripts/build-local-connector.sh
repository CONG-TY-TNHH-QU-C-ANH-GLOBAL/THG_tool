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

  package_kit() {
    local binary="$1"
    local kit_name="$2"
    local runtime_name="$3"
    local kit_root="$OUTPUT_DIR/.kit-${kit_name%.zip}"
    rm -rf "$kit_root" "$OUTPUT_DIR/$kit_name"
    mkdir -p "$kit_root"
    cp "$OUTPUT_DIR/$binary" "$kit_root/$runtime_name"
    cat > "$kit_root/README.txt" <<EOF
THG Local Kit

This package contains:
- THG Local Runtime: $runtime_name

Production flow:
1. Open the THG Browser dashboard.
2. Create a new pairing code.
3. Run the THG Local Runtime and paste the pairing code.
4. Keep the Runtime open, then click "Chay Facebook" in the dashboard.

Security:
- Do not enter your Facebook password into THG.
- Runtime runs isolated local Chrome profiles and streams them to the dashboard.
EOF
    echo "Packaging $kit_name"
    if command -v python3 >/dev/null 2>&1; then
      (cd "$kit_root" && python3 -m zipfile -c "$OUTPUT_DIR/$kit_name" .)
    else
      (cd "$kit_root" && zip -qr "$OUTPUT_DIR/$kit_name" .)
    fi
    chmod 0644 "$OUTPUT_DIR/$kit_name"
    rm -rf "$kit_root"
  }

  package_kit thg-login-windows.exe thg-local-kit-windows.zip THG-Local-Runtime.exe
  package_kit thg-login-linux thg-local-kit-linux.zip thg-local-runtime
  package_kit thg-login-mac-intel thg-local-kit-mac-intel.zip thg-local-runtime
  package_kit thg-login-mac-m1 thg-local-kit-mac-m1.zip thg-local-runtime

  if [ -d "$ROOT/frontend" ]; then
    mkdir -p "$ROOT/frontend/public/downloads"
    cp "$OUTPUT_DIR/thg-chrome-extension.zip" "$ROOT/frontend/public/downloads/thg-chrome-extension.zip"
    cp "$OUTPUT_DIR"/thg-local-kit-*.zip "$ROOT/frontend/public/downloads/"
  fi
fi
echo "Local Connector builds written to $OUTPUT_DIR"
