#!/usr/bin/env bash
# Publishes the latest lomod/lomoc build (from releases/, produced by
# build.sh + amd64/build.sh + package.sh, or build-all.sh) into the
# lomoware.github.io apt repo via reprepro.
#
# Debian family (buster/bullseye/bookworm/trixie) reuses the one
# buster-linked binary -- their vips/glibc SONAMEs have stayed stable
# release over release (confirmed: byte-identical lomo-vips blob across
# buster->bookworm->trixie). jammy/noble get their own freshly-built
# lomo-vips too (Ubuntu bumps SONAMEs on its own schedule; this was
# confirmed the hard way -- lomod failed to even start on jammy/noble
# against the Debian-built lomo-vips before per-release builds existed).
#
# Deliberately NOT touched here: jessie/groovy (not in current install
# docs, no longer actively supported) and bionic/focal (still documented,
# but their lomo-vips packages were built via older, separate infra and
# have never been smoke-tested against a fresh lomod build the way
# jammy/noble's now have -- pushing a new lomod there without that check
# would risk repeating the exact SONAME bug jammy/noble hit).
set -ex

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RELEASES="$REPO/releases"
SITE=/mnt/c/Users/fuji2/Work/lomoware.github.io/debian

if [ ! -f "$RELEASES/LATEST_RELEASE_RPI" ]; then
  echo "no build found; run build.sh/amd64/build.sh/package.sh (or build-all.sh) first" >&2
  exit 1
fi
LOMOD_VERSION=$(cat "$RELEASES/LATEST_RELEASE_RPI")

# lomo-vips itself isn't republished here: its version (8.12.2) is unchanged
# and jammy/noble already have it from when it was first added; reprepro
# would reject a same-version-different-checksum resubmit anyway. Bump
# VIPS_VERSION and rerun build-vips-ubuntu.sh first if vips itself changed.
for f in "$RELEASES/lomod_${LOMOD_VERSION}_armhf.deb" \
         "$RELEASES/lomod_${LOMOD_VERSION}_arm64.deb" \
         "$RELEASES/lomod_${LOMOD_VERSION}_amd64.deb" \
         "$RELEASES/lomo-backend-docker_${LOMOD_VERSION}_armhf.deb" \
         "$RELEASES/lomo-backend-docker_${LOMOD_VERSION}_arm64.deb" \
         "$RELEASES/lomo-backend-docker_${LOMOD_VERSION}_amd64.deb"; do
  [ -f "$f" ] || { echo "missing $f" >&2; exit 1; }
done

for dist in buster bullseye bookworm trixie; do
  (
    cd "$SITE/$dist"
    reprepro -P 1 -S main includedeb "$dist" "$RELEASES/lomod_${LOMOD_VERSION}_armhf.deb"
    reprepro -P 1 -S main includedeb "$dist" "$RELEASES/lomod_${LOMOD_VERSION}_arm64.deb"
    reprepro -P 1 -S main includedeb "$dist" "$RELEASES/lomod_${LOMOD_VERSION}_amd64.deb"
    # lomo-backend-docker (package-docker.sh): what lomo-docker's Dockerfiles install when
    # built without a local deb
    for arch in armhf arm64 amd64; do
      reprepro -P 1 -S main includedeb "$dist" "$RELEASES/lomo-backend-docker_${LOMOD_VERSION}_$arch.deb"
    done
  )
done

for dist in jammy noble; do
  (
    cd "$SITE/$dist"
    reprepro -P 1 -S main includedeb "$dist" "$RELEASES/lomod_${LOMOD_VERSION}_amd64.deb"
  )
done

echo "Published lomod ${LOMOD_VERSION} to buster/bullseye/bookworm/trixie/jammy/noble, lomo-backend-docker to buster/bullseye/bookworm/trixie."
