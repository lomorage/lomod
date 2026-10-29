#!/usr/bin/env bash
# Publishes the lomo-vips debs in releases/ into the lomoware.github.io apt
# repo via reprepro. Separate from publish-debian.sh, which publishes lomod:
# lomo-vips moves on its own, much slower schedule, and the great majority of
# lomod releases must not touch it -- the same reason publish-rocky9.sh and
# publish-opensuse.sh are their own scripts.
#
# It exists because lomo-vips did change: it now carries a libheif that can
# decode AVIF, see amd64/Dockerfile.debian-vips-build. vips itself is still
# 8.12.2, so the change shows up as a bumped Debian revision (PKG_REVISION),
# which is also what makes it publishable -- reprepro rejects a
# same-version-different-checksum resubmit, and apt on an already-installed
# device would not have seen an upgrade at plain 8.12.2 either.
#
# Publishes whichever debs are actually present and reports the rest as
# skipped, rather than demanding the full codename/arch matrix: a codename
# whose own libheif already decodes AVIF (bullseye upward) has nothing new to
# ship, so those are normally not rebuilt. Fails if it finds nothing at all.
#
# Run from WSL2, after amd64/build-vips-debian.sh / amd64/build-vips-ubuntu.sh.
set -eu

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RELEASES="$REPO/releases"
SITE=/mnt/c/Users/fuji2/Work/lomoware.github.io/debian
PKG_VERSION="${PKG_VERSION:-8.12.2-2}"

published=0
skipped=""
for dist in buster bullseye bookworm trixie jammy noble; do
  for arch in armhf arm64 amd64; do
    deb="$RELEASES/lomo-vips_${PKG_VERSION}_${dist}_${arch}.deb"
    if [ ! -f "$deb" ]; then
      skipped="$skipped ${dist}/${arch}"
      continue
    fi
    (
      set -x
      cd "$SITE/$dist"
      reprepro -P 1 -S main includedeb "$dist" "$deb"
    )
    published=$((published + 1))
  done
done

if [ "$published" -eq 0 ]; then
  echo "no lomo-vips_${PKG_VERSION}_*.deb in $RELEASES" >&2
  echo "build them first, e.g. amd64/build-vips-debian.sh buster debian:buster true arm64" >&2
  exit 1
fi

echo "published $published lomo-vips ${PKG_VERSION} deb(s)"
if [ -n "$skipped" ]; then
  echo "not present, skipped:$skipped"
fi
