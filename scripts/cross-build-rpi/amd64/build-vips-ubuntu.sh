#!/usr/bin/env bash
# Builds + packages lomo-vips from source for a given Ubuntu codename.
# Needed because Ubuntu bumps webp/tiff/ImageMagick to SONAMEs incompatible
# with the Debian-buster-built lomo-vips (confirmed: lomod fails to even
# start against it on jammy/noble with "cannot open shared object file").
# Usage: build-vips-ubuntu.sh jammy ubuntu:22.04
#        build-vips-ubuntu.sh noble ubuntu:24.04
set -ex

CODENAME="$1"
BASE_IMAGE="$2"
VIPS_VERSION="${VIPS_VERSION:-8.12.2}"

if [ -z "$CODENAME" ] || [ -z "$BASE_IMAGE" ]; then
  echo "usage: $0 <codename> <base-image>" >&2
  echo "  e.g. $0 jammy ubuntu:22.04" >&2
  exit 1
fi

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
IMG="lomo-vips-${CODENAME}-build"
OUT="$REPO/releases"
mkdir -p "$OUT"

docker build --build-arg "BASE_IMAGE=$BASE_IMAGE" --build-arg "VIPS_VERSION=$VIPS_VERSION" \
  -t "$IMG" -f "$REPO/scripts/cross-build-rpi/amd64/Dockerfile.ubuntu-vips-build" \
  "$REPO/scripts/cross-build-rpi/amd64"

# -dev packages so apt pulls in each release's own matching runtime libs.
DEPENDS="liblcms2-dev,libexpat1-dev,libfftw3-dev,libwebp-dev,libtiff-dev,libexif-dev,libpng-dev,libjpeg-turbo8-dev,libglib2.0-dev,libimagequant-dev,libheif-dev,liborc-0.4-dev,libmagickcore-dev,libxml2-dev,libde265-dev,libopenjp2-7-dev,libsqlite3-dev,libgsf-1-dev,imagemagick"

dist=$(mktemp -d)
mkdir -p "$dist/usr/local" "$dist/DEBIAN"

cid=$(docker create "$IMG")
docker cp "$cid:/usr/local/." "$dist/usr/local/"
docker rm "$cid" >/dev/null

cat > "$dist/DEBIAN/control" <<EOF
Package: lomo-vips
Version: $VIPS_VERSION
Maintainer: Lomoware
Architecture: amd64
Description: libvips package required by lomorage
Depends: $DEPENDS
EOF

outname="lomo-vips_${VIPS_VERSION}_${CODENAME}_amd64.deb"
# -Zxz for consistency with build-vips-debian.sh/package.sh (jammy/noble's
# own dpkg supports zstd, but there's no reason to differ here).
dpkg-deb --build -Zxz --root-owner-group "$dist" "$OUT/$outname"
rm -rf "$dist"

echo "built $OUT/$outname"
