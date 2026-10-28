#!/usr/bin/env bash
# Builds every published lomod/lomoc/lomo-vips artifact, across every
# platform this project currently supports, in one shot:
#   - lomod/lomoc for arm64/armhf (cross-gcc) and amd64 (native), linked
#     against Debian buster's glibc -- covers Debian 10-13 and reused as-is
#     for Ubuntu bionic/focal/groovy (shared SONAMEs, see README.md)
#   - .deb packages for the above (armhf/arm64/amd64)
#   - lomo-vips for Ubuntu jammy/noble (SONAME-incompatible with Debian,
#     needs its own from-source build)
#   - RPMs for Fedora (reuses the amd64 binaries + Fedora's own vips),
#     Rocky Linux 9, and openSUSE Leap (both need from-source lomo-vips)
#
# Does NOT sign or publish anything -- that touches the live repo/signing
# key and is a deliberate separate step, see publish-fedora.sh /
# publish-rocky9.sh / publish-opensuse.sh and the debian/ apt repo tooling
# in lomoware.github.io. This script only fills releases/.
#
# Run from WSL2 (needs docker, plus the one-time cross-gcc/sysroot setup
# in README.md's "One-time setup" section).
set -ex

cd "$(dirname "${BASH_SOURCE[0]}")"
REPO="$(cd .. && cd .. && pwd)"

build_image_if_missing() {
  local image="$1" dockerfile="$2" context="$3"
  docker image inspect "$image" >/dev/null 2>&1 || \
    docker build -t "$image" -f "$dockerfile" "$context"
}

# --- Debian-family binaries ---
bash build.sh arm64
bash build.sh armhf
build_image_if_missing lomod-amd64-buster-build amd64/Dockerfile.buster-build amd64
bash amd64/build.sh

# --- .deb packaging (armhf/arm64/amd64) ---
bash package.sh
# lomo-backend-docker debs for the lomorage/lomo-docker images, same binaries
bash package-docker.sh

# --- lomo-vips for Ubuntu jammy/noble ---
bash amd64/build-vips-ubuntu.sh jammy ubuntu:22.04
bash amd64/build-vips-ubuntu.sh noble ubuntu:24.04

# --- RPM: Fedora ---
bash rpm/package-fedora.sh

# --- RPM: Rocky Linux 9 ---
build_image_if_missing lomo-vips-rocky9-build rpm/Dockerfile.rocky9-vips-build rpm
bash rpm/package-vips-generic.sh lomo-vips-rocky9-build rockylinux:9 \
  lomo-vips_8.12.2_rocky9_amd64.rpm \
  "glib2-devel, expat-devel, fftw-devel, ImageMagick-devel, orc-devel, lcms2-devel, libheif-devel, zlib-devel, libwebp-devel, libtiff-devel, libpng-devel, libimagequant-devel, libjpeg-turbo-devel, libexif-devel, libgsf-devel, libxml2-devel, openjpeg2-devel, sqlite-devel"
bash rpm/package-rocky9.sh

# --- RPM: openSUSE Leap ---
# Pinned, not :latest: upstream slid that tag from 15.x to 16.0, silently changing which release
# our RPMs target. The same tag is in rpm/Dockerfile.opensuse-vips-build and rpm/package-opensuse.sh,
# and all three must agree -- lomo-vips and lomod have to be built against the same Leap.
build_image_if_missing lomo-vips-opensuse-build rpm/Dockerfile.opensuse-vips-build rpm
# libheif-aom: Leap 16.0's libheif is 1.23.5, new enough for AVIF, but it ships
# the AV1 decoder as a separate dlopen'd plugin package, so the auto-Requires on
# vips's linked SONAMEs never pulls it and heif_have_decoder_for_format(AV1) comes
# back false -- an .avif stores but gets no preview. Measured, not assumed: Rocky 9's
# libheif 1.23.6 already answers true without any extra package.
bash rpm/package-vips-generic.sh lomo-vips-opensuse-build opensuse/leap:16.0 \
  lomo-vips_8.12.2_opensuse_amd64.rpm \
  "sqlite3, libblkid1, util-linux, libheif-aom"
bash rpm/package-opensuse.sh

echo "=== Done. Artifacts: ==="
ls -la "$REPO/releases"
