#!/usr/bin/env bash
# Assembles Frictionless-Launcher.app from an already-built binary and
# ad-hoc signs it. macOS-only (uses sips/iconutil/codesign). Used by both
# `make mac-app` (local dev) and release.yml's build-darwin job, so bundle
# logic only lives in one place.
#
# Usage: build-app.sh <path-to-binary> <output-dir> [version]
set -euo pipefail

BIN="$1"
OUT_DIR="$2"
VERSION="${3:-0.0.0-dev}"

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$HERE/../.." && pwd)"
ICON="$REPO_ROOT/internal/assets/icon.png"
BUNDLE="$OUT_DIR/Frictionless-Launcher.app"

rm -rf "$BUNDLE"
mkdir -p "$BUNDLE/Contents/MacOS" "$BUNDLE/Contents/Resources"
cp "$BIN" "$BUNDLE/Contents/MacOS/Frictionless-Launcher"
chmod +x "$BUNDLE/Contents/MacOS/Frictionless-Launcher"

sed "s/{{VERSION}}/$VERSION/" "$HERE/Info.plist" > "$BUNDLE/Contents/Info.plist"

ICONSET_DIR="$(mktemp -d)/icon.iconset"
mkdir -p "$ICONSET_DIR"
for size in 16 32 128 256 512; do
  sips -z "$size" "$size" "$ICON" --out "$ICONSET_DIR/icon_${size}x${size}.png" >/dev/null
  double=$((size * 2))
  sips -z "$double" "$double" "$ICON" --out "$ICONSET_DIR/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$ICONSET_DIR" -o "$BUNDLE/Contents/Resources/icon.icns"

codesign -s - --deep --force "$BUNDLE"

echo "Built $BUNDLE"
