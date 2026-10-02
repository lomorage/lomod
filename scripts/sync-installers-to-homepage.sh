#!/usr/bin/env bash
# Copies the one-line CLI installers from this repo -- their single source of truth -- into the
# homepage repo, which serves them as https://lomorage.com/windows/install.ps1 and
# https://lomorage.com/mac/install.sh. Edit them here, never in homepage: this overwrites
# homepage's copies byte for byte.
#
# Also updates homepage's migration/download-assets.json, which pins each served file's sha256.
# homepage's CI runs `npm test` (which checks those hashes) before deploying, so a copied
# installer without its new hash would block the whole site deploy. Then runs that test suite.
#
# Copies the committed version (`git cat-file blob HEAD:<path>`), not the working tree, and
# refuses if the installers have uncommitted changes: this checkout has core.autocrlf=true, so
# a working-tree file can be CRLF at any time, and CRLF would break `curl | bash` on macOS and
# not match what CI builds from a Linux checkout.
#
# Does not commit or push homepage. scripts/release-all.sh runs this before publishing anything
# and commits the result together with its release.json bump. For an installer-only change, run
# this, then review and commit in homepage yourself.
#
# Usage: scripts/sync-installers-to-homepage.sh [--check] [--no-test]
#   --check     Only report whether homepage's copies match this repo's HEAD; exit 1 if not.
#   --no-test   Skip homepage's `npm test`.
#   HOMEPAGE_DIR env var overrides the homepage checkout (default: ../homepage next to this repo).
#
# Needs node on PATH (homepage's own toolchain) -- on this setup that means native Windows
# git-bash, not WSL.
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HOMEPAGE="${HOMEPAGE_DIR:-$REPO/../homepage}"
CHECK=0
RUN_TESTS=1
for arg in "$@"; do
    case "$arg" in
        --check) CHECK=1 ;;
        --no-test) RUN_TESTS=0 ;;
        *) echo "unknown argument: $arg" >&2; exit 2 ;;
    esac
done

# <path in this repo>:<path under homepage/static, also its "path" in download-assets.json>
PAIRS=(
    "installers/windows/install.ps1:windows/install.ps1"
    "installers/macos/install.sh:mac/install.sh"
)

[ -d "$HOMEPAGE/static" ] || { echo "homepage checkout not found at $HOMEPAGE" >&2; exit 1; }
MANIFEST="$HOMEPAGE/migration/download-assets.json"
COMMIT="$(git -C "$REPO" rev-parse HEAD)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

drift=0
for pair in "${PAIRS[@]}"; do
    src="${pair%%:*}"
    dst="${pair##*:}"
    git -C "$REPO" cat-file blob "HEAD:$src" > "$TMP/src"
    if cmp -s "$TMP/src" "$HOMEPAGE/static/$dst"; then
        echo "in sync: static/$dst"
    else
        echo "differs: static/$dst (lomo-backend HEAD:$src)"
        drift=1
    fi
done
if [ "$CHECK" = 1 ]; then
    exit "$drift"
fi

for pair in "${PAIRS[@]}"; do
    src="${pair%%:*}"
    if ! git -C "$REPO" diff --quiet HEAD -- "$src"; then
        echo "$src has uncommitted changes -- commit it first, so homepage gets exactly what's in lomo-backend's history" >&2
        exit 1
    fi
done

command -v node >/dev/null || { echo "node not found on PATH (needed to update $MANIFEST and run homepage's tests)" >&2; exit 1; }

node_args=()
for pair in "${PAIRS[@]}"; do
    src="${pair%%:*}"
    dst="${pair##*:}"
    git -C "$REPO" cat-file blob "HEAD:$src" > "$HOMEPAGE/static/$dst"
    node_args+=("$dst" "$src" "$(git -C "$REPO" rev-parse "HEAD:$src")" "$(sha256sum "$HOMEPAGE/static/$dst" | cut -d' ' -f1)")
done

node - "$MANIFEST" "$COMMIT" "${node_args[@]}" <<'EOF'
const fs = require('fs');
const [manifestPath, commit, ...rest] = process.argv.slice(2);
const data = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));
for (let i = 0; i < rest.length; i += 4) {
  const [path, src, blob, sha256] = rest.slice(i, i + 4);
  const entry = data.files.find(f => f.path === path);
  if (!entry) throw new Error(`${path} not listed in ${manifestPath}`);
  // Top-level "source"/"sourceTree" still describe the original download-site migration for the
  // other assets; these per-file fields say where the installers now come from.
  entry.source = `lomorage/lomod@${commit.slice(0, 7)}:${src}`;
  entry.sourceBlob = blob;
  entry.sourceSha256 = sha256;
  entry.sha256 = sha256;
}
fs.writeFileSync(manifestPath, JSON.stringify(data, null, 2) + '\n');
EOF

echo
git -C "$HOMEPAGE" status --short -- static/windows/install.ps1 static/mac/install.sh migration/download-assets.json

if [ "$RUN_TESTS" = 1 ]; then
    [ -d "$HOMEPAGE/node_modules" ] || { echo "homepage has no node_modules -- run 'npm ci' in $HOMEPAGE first" >&2; exit 1; }
    (cd "$HOMEPAGE" && npm test)
fi
