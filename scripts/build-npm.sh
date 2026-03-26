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
    go build -ldflags="-s -w -X github.com/iamdecatalyst/hummingbird/cli/tui.Version=v$VERSION" \
    -o "$OUT" .)

  # Make Unix binaries executable
  if [[ "$BINARY" != *.exe ]]; then
    chmod +x "$OUT"
  fi
done

# Update version in all package.json files using Python (sed/perl mangle key names)
echo ""
echo "▶ Updating version to $VERSION in package.json files"

python3 - "$NPM" "$VERSION" <<'PYEOF'
import json, sys, pathlib

npm_dir = pathlib.Path(sys.argv[1])
version = sys.argv[2]

PLATFORM_PKGS = [
  "@decatalyst/hummingbird-linux-x64",
  "@decatalyst/hummingbird-linux-arm64",
  "@decatalyst/hummingbird-darwin-x64",
  "@decatalyst/hummingbird-darwin-arm64",
  "@decatalyst/hummingbird-win32-x64",
]

for pkg_json in npm_dir.rglob("package.json"):
  with open(pkg_json) as f:
    pkg = json.load(f)
  pkg["version"] = version
  if "optionalDependencies" in pkg:
    pkg["optionalDependencies"] = {p: version for p in PLATFORM_PKGS}
  with open(pkg_json, "w") as f:
    json.dump(pkg, f, indent=2, ensure_ascii=False)
    f.write("\n")
  print(f"  updated {pkg_json.relative_to(npm_dir.parent.parent)}")
PYEOF

echo ""
echo "✓ Build complete. Run ./scripts/publish-npm.sh $VERSION to publish."
