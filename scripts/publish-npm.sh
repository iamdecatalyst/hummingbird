#!/usr/bin/env bash
# Publish all Hummingbird npm packages in the correct order.
# Run AFTER build-npm.sh: ./scripts/publish-npm.sh [version] [--dry-run]
set -euo pipefail

VERSION="${1:-1.0.0}"
DRY="${2:-}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NPM="$ROOT/npm/@iamdecatalyst"

FLAGS="--access public"
if [[ "$DRY" == "--dry-run" ]]; then
  FLAGS="$FLAGS --dry-run"
  echo "▶ DRY RUN — no packages will actually be published"
fi

echo "▶ Publishing @iamdecatalyst/hummingbird@$VERSION"
echo ""

# Publish platform packages first — main package depends on them
PLATFORM_PKGS=(
  hummingbird-linux-x64
  hummingbird-linux-arm64
  hummingbird-darwin-x64
  hummingbird-darwin-arm64
  hummingbird-win32-x64
)

for PKG in "${PLATFORM_PKGS[@]}"; do
  echo "  publishing @iamdecatalyst/$PKG..."
  cd "$NPM/$PKG"
  npm publish $FLAGS
done

# Main package last
echo "  publishing @iamdecatalyst/hummingbird..."
cd "$NPM/hummingbird"
npm publish $FLAGS

echo ""
echo "✓ Published @iamdecatalyst/hummingbird@$VERSION"
echo ""
echo "  Install:  npm install -g @iamdecatalyst/hummingbird"
echo "  Run:      npx @iamdecatalyst/hummingbird"
echo "  Alias:    npm install -g @iamdecatalyst/hummingbird && hummingbird"
