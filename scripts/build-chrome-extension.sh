#!/usr/bin/env bash
#
# Package the THG Chrome Extension for distribution.
#
# Usage:
#   build-chrome-extension.sh [VERSION_SUFFIX]
#
# VERSION_SUFFIX behavior:
#   - "dev" / unset / "0" → manifest.json is zipped as-is (base version, e.g. "0.3.0").
#     Use this for local dev where you don't want to keep bumping versions.
#   - numeric (e.g. CI ${{ github.run_number }} = "42") → manifest is patched to
#     "<base>.<suffix>" (e.g. "0.3.0.42") in the staged copy ONLY. The repo's
#     manifest.json is never touched.
#   - non-numeric (e.g. git SHA "6283361") → first 4 hex chars are converted to
#     a decimal in [0, 65535] and used as the suffix. Chrome's manifest spec
#     caps each version segment at 65535, hence the modulo.
#
# Why a 4th segment instead of bumping minor:
#   Chrome treats a higher version as an upgrade and reloads code on next sync.
#   Adding ".${run_number}" guarantees every CI build looks fresh to Chrome
#   without touching the human-meaningful base (major.minor.patch). Devs still
#   see "0.3.0" in source — the build suffix only exists in shipped artifacts.

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

# Stage build in a temp dir so manifest patching never dirties the working
# tree. trap cleans up even if the zip step fails.
STAGING_DIR="$(mktemp -d)"
trap 'rm -rf "$STAGING_DIR"' EXIT
cp -R "$EXTENSION_DIR/." "$STAGING_DIR/"

stamp_manifest_version() {
  local suffix="$1"
  if ! command -v python3 >/dev/null 2>&1; then
    echo "python3 required to stamp manifest version (got VERSION=$VERSION)" >&2
    return 1
  fi
  python3 - "$STAGING_DIR/manifest.json" "$suffix" <<'PY'
import json
import sys

manifest_path, suffix = sys.argv[1], sys.argv[2]
with open(manifest_path, encoding="utf-8") as f:
    manifest = json.load(f)
base = manifest.get("version", "0.0.0")
parts = base.split(".")
# Chrome caps versions at 4 dotted segments; truncate to 3 before appending so
# we don't blow past the limit if someone manually committed a 4-part version.
if len(parts) > 3:
    parts = parts[:3]
manifest["version"] = ".".join(parts + [suffix])
with open(manifest_path, "w", encoding="utf-8") as f:
    json.dump(manifest, f, indent=2)
    f.write("\n")
print(f"Stamped extension version: {manifest['version']}")
PY
}

if [ -n "$VERSION" ] && [ "$VERSION" != "dev" ] && [ "$VERSION" != "0" ]; then
  if [[ "$VERSION" =~ ^[0-9]+$ ]]; then
    BUILD_SUFFIX="$VERSION"
  else
    # Hash-style input (git SHA): convert first 4 hex chars to int. Strips a
    # "v" prefix if someone passed a tag like "v1.2".
    HEX="${VERSION#v}"
    HEX="${HEX:0:4}"
    if [[ "$HEX" =~ ^[0-9a-fA-F]+$ ]]; then
      BUILD_SUFFIX=$((16#${HEX}))
    else
      echo "Cannot derive numeric build suffix from VERSION='$VERSION'; falling back to 0" >&2
      BUILD_SUFFIX=0
    fi
  fi
  # Chrome rejects segments > 65535. Modulo keeps a sortable-ish suffix.
  if [ "$BUILD_SUFFIX" -gt 65535 ]; then
    BUILD_SUFFIX=$((BUILD_SUFFIX % 65536))
  fi
  stamp_manifest_version "$BUILD_SUFFIX"
fi

echo "Packaging THG Chrome Extension $VERSION"
rm -f "$OUTPUT_DIR/$ZIP_NAME"
if command -v python3 >/dev/null 2>&1; then
  (cd "$STAGING_DIR" && python3 -m zipfile -c "$OUTPUT_DIR/$ZIP_NAME" .)
else
  (cd "$STAGING_DIR" && zip -qr "$OUTPUT_DIR/$ZIP_NAME" .)
fi
chmod 0644 "$OUTPUT_DIR/$ZIP_NAME"

echo "Chrome Extension package written to $OUTPUT_DIR/$ZIP_NAME"
