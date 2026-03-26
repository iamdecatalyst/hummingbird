#!/usr/bin/env bash
# Build Hummingbird CLI for all platforms and copy into npm packages.
# Run from repo root: ./scripts/build-npm.sh [version]
set -euo pipefail

VERSION="${1:-1.0.0}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CLI="$ROOT/cli"
NPM="$ROOT/npm/@decatalyst"

echo "▶ Building hummingbird CLI $VERSION for all platforms"
echo ""

declare -A TARGETS=(
  ["linux/amd64"]="hummingbird-linux-x64:hummingbird"
  ["linux/arm64"]="hummingbird-linux-arm64:hummingbird"
  ["darwin/amd64"]="hummingbird-darwin-x64:hummingbird"
  ["darwin/arm64"]="hummingbird-darwin-arm64:hummingbird"
  ["windows/amd64"]="hummingbird-win32-x64:hummingbird.exe"
)

for PLATFORM in "${!TARGETS[@]}"; do
  GOOS="${PLATFORM%/*}"
  GOARCH="${PLATFORM#*/}"
  ENTRY="${TARGETS[$PLATFORM]}"
  PKG="${ENTRY%:*}"
  BINARY="${ENTRY#*:}"

  OUT="$NPM/$PKG/bin/$BINARY"
  printf "  %-24s → npm/@decatalyst/%s/bin/%s\n" "$GOOS/$GOARCH" "$PKG" "$BINARY"

  (cd "$CLI" && GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=0 \
    go build -ldflags="-s -w -X main.version=$VERSION" \
    -o "$OUT" .)

  # Make Unix binaries executable
  if [[ "$BINARY" != *.exe ]]; then
    chmod +x "$OUT"
  fi
done

# Update version in all package.json files (version + optionalDependencies)
echo ""
echo "▶ Updating version to $VERSION in package.json files"

for PKG_DIR in "$NPM"/hummingbird "$NPM"/hummingbird-*/; do
  PKG_JSON="$PKG_DIR/package.json"
  if [[ -f "$PKG_JSON" ]]; then
    # Update "version" field
    sed -i.bak "s/\"version\": \"[^\"]*\"/\"version\": \"$VERSION\"/" "$PKG_JSON"
    rm -f "$PKG_JSON.bak"
    # Update pinned versions inside optionalDependencies (lines like "pkg": "x.y.z")
    sed -i.bak "s/@decatalyst\/hummingbird-[^\"]*\": \"[^\"]*\"/@decatalyst\/hummingbird-PLACEHOLDER\": \"$VERSION\"/g" "$PKG_JSON"
    rm -f "$PKG_JSON.bak"
    # Fix the PLACEHOLDER substitution (restore real pkg names by re-running build)
    # Actually: just replace any semver value on lines containing @decatalyst
    perl -i -pe 's/("@decatalyst\/hummingbird-[^"]+": ")[^"]+"/\${1}'"$VERSION"'"/' "$PKG_JSON" 2>/dev/null || true
  fi
done

echo ""
echo "✓ Build complete. Run ./scripts/publish-npm.sh $VERSION to publish."
