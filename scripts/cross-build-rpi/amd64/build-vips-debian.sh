#!/usr/bin/env bash
# Builds + packages lomo-vips from source for a given Debian codename and
# arch (amd64 by default; arm64/armhf build under qemu via docker
# --platform). See Dockerfile.debian-vips-build for why this exists: the
# previously-published amd64 lomo-vips for bookworm/trixie was linked
# against buster-era SONAMEs that don't exist there, and buster/bullseye
# never had one at all -- each Debian codename now gets its own build, same
# discipline as build-vips-ubuntu.sh for jammy/noble.
# Usage: build-vips-debian.sh buster debian:buster true
#        build-vips-debian.sh bullseye debian:bullseye
#        build-vips-debian.sh bookworm debian:bookworm
#        build-vips-debian.sh trixie debian:trixie
#        build-vips-debian.sh trixie debian:trixie false arm64
set -ex

CODENAME="$1"
BASE_IMAGE="$2"
USE_ARCHIVE_MIRROR="${3:-false}"
ARCH="${4:-amd64}"
VIPS_VERSION="${VIPS_VERSION:-8.12.2}"

case "$ARCH" in
  amd64) PLATFORM=linux/amd64 ;;
  arm64) PLATFORM=linux/arm64 ;;
  armhf) PLATFORM=linux/arm/v7 ;;
  *) echo "unsupported arch: $ARCH" >&2; exit 1 ;;
esac

if [ -z "$CODENAME" ] || [ -z "$BASE_IMAGE" ]; then
  echo "usage: $0 <codename> <base-image> [use-archive-mirror] [amd64|arm64|armhf]" >&2
  echo "  e.g. $0 bookworm debian:bookworm" >&2
  exit 1
fi

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
IMG="lomo-vips-debian-${CODENAME}-${ARCH}-build"
OUT="$REPO/releases"
mkdir -p "$OUT"

docker build --platform "$PLATFORM" --build-arg "BASE_IMAGE=$BASE_IMAGE" --build-arg "VIPS_VERSION=$VIPS_VERSION" \
  --build-arg "USE_ARCHIVE_MIRROR=$USE_ARCHIVE_MIRROR" \
  -t "$IMG" -f "$REPO/scripts/cross-build-rpi/amd64/Dockerfile.debian-vips-build" \
  "$REPO/scripts/cross-build-rpi/amd64"

# -dev packages so apt pulls in each release's own matching runtime libs.
DEPENDS="liblcms2-dev,libexpat1-dev,libfftw3-dev,libwebp-dev,libtiff-dev,libexif-dev,libpng-dev,libjpeg62-turbo-dev,libglib2.0-dev,libimagequant-dev,libheif-dev,liborc-0.4-dev,libmagickcore-dev,libxml2-dev,libde265-dev,libopenjp2-7-dev,libsqlite3-dev,libgsf-1-dev,imagemagick"

dist=$(mktemp -d)
mkdir -p "$dist/usr/local" "$dist/DEBIAN"

cid=$(docker create --platform "$PLATFORM" "$IMG")
docker cp "$cid:/usr/local/." "$dist/usr/local/"
docker rm "$cid" >/dev/null

cat > "$dist/DEBIAN/control" <<EOF
Package: lomo-vips
Version: $VIPS_VERSION
Maintainer: Lomoware
Architecture: $ARCH
Description: libvips package required by lomorage
Depends: $DEPENDS
EOF

outname="lomo-vips_${VIPS_VERSION}_${CODENAME}_${ARCH}.deb"
# -Zxz: modern dpkg-deb defaults to zstd, which buster/bullseye's dpkg
# (1.19/1.20) can't unpack at all -- xz is supported since ancient dpkg.
dpkg-deb --build -Zxz --root-owner-group "$dist" "$OUT/$outname"
rm -rf "$dist"

echo "built $OUT/$outname"
