#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-${VERSION:-dev}}"
OUT_DIR="${OUT_DIR:-dist/chrome-web-store}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR="$ROOT/$OUT_DIR"
EXTENSION_DIR="$ROOT/local-connector-extension"
ZIP_NAME="thg-chrome-extension.zip"

mkdir -p "$OUTPUT_DIR"

if [ ! -d "$EXTENSION_DIR" ]; then
  echo "Extension directory not found: $EXTENSION_DIR" >&2
  exit 1
fi

echo "Packaging THG Chrome Extension $VERSION"
rm -f "$OUTPUT_DIR/$ZIP_NAME"
if command -v python3 >/dev/null 2>&1; then
  (cd "$EXTENSION_DIR" && python3 -m zipfile -c "$OUTPUT_DIR/$ZIP_NAME" .)
else
  (cd "$EXTENSION_DIR" && zip -qr "$OUTPUT_DIR/$ZIP_NAME" .)
fi
chmod 0644 "$OUTPUT_DIR/$ZIP_NAME"

echo "Chrome Extension package written to $OUTPUT_DIR/$ZIP_NAME"
