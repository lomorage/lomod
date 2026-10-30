#!/usr/bin/env bash
# Builds every published lomod/lomoc/lomo-vips artifact, across every
# platform this project currently supports, in one shot:
#   - lomod/lomoc for arm64/armhf (cross-gcc) and amd64 (native), linked
#     against Debian buster's glibc -- covers Debian 10-13 and reused as-is
#     for Ubuntu bionic/focal/groovy (shared SONAMEs, see README.md)
#   - .deb packages for the above (armhf/arm64/amd64)
#   - lomo-vips for Debian buster/bullseye/bookworm/trixie (amd64; the
#     arm64/armhf builds buster and bookworm need are opt-in via
#     BUILD_VIPS_QEMU=1, they run under qemu at half an hour each)
#   - lomo-vips for Ubuntu jammy/noble (SONAME-incompatible with Debian,
#     needs its own from-source build)
#   - RPMs for Fedora (reuses the amd64 binaries + Fedora's own vips),
#     Rocky Linux 9, and openSUSE Leap (both need from-source lomo-vips)
#
# Does NOT sign or publish anything -- that touches the live repo/signing
# key and is a deliberate separate step, see publish-fedora.sh /
# publish-rocky9.sh / publish-opensuse.sh, publish-debian.sh for lomod and
# publish-vips-debian.sh for lomo-vips, plus the debian/ apt repo tooling in
# lomoware.github.io. This script only fills releases/.
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

# --- lomo-vips for Debian buster/bullseye/bookworm/trixie ---
# Each codename needs its own build, see amd64/Dockerfile.debian-vips-build:
# the amd64 lomo-vips once published for bookworm/trixie was buster-linked and
# would not load there. amd64 is native and takes minutes.
for cn in buster bullseye bookworm trixie; do
  # buster's own mirrors are EOL, so it needs archive.debian.org.
  if [ "$cn" = buster ]; then archive=true; else archive=false; fi
  bash amd64/build-vips-debian.sh "$cn" "debian:$cn" "$archive" amd64
done
# arm64/armhf run every compiler process under qemu and take half an hour
# each, so they are opt-in rather than part of every release. Which codenames
# need them was measured by installing the *published* deb in a container of
# its own release and trying to decode:
#   buster   arm64/armhf -- vips runs, AVIF does not decode. The published
#            deb bundles a libheif 1.12.0 under /usr/local/lib, but on arm the
#            ldconfig cache prefers /usr/lib/<triplet> (aarch64-linux-gnu.conf
#            sorts before libc.conf), so libvips loads buster's own 1.3.2.
#   bookworm arm64/armhf -- vips does not even start:
#            "libwebp.so.6: cannot open shared object file". The published deb
#            is the buster-built blob, and bookworm has libwebp.so.7. This is
#            the same breakage that was found and fixed for amd64, still live
#            on arm, so lomod cannot run there at all today.
#   bullseye arm64/armhf -- fine as published: still has libwebp.so.6, and its
#            own libheif 1.11.0 decodes AVIF.
#   trixie   arm64/armhf -- fine, already built per-release.
# Run with BUILD_VIPS_QEMU=1 when that is what you actually need. MAKE_JOBS
# caps parallelism; the default nproc gets the build OOM-killed under qemu on
# a small box.
if [ "${BUILD_VIPS_QEMU:-0}" = "1" ]; then
  for arch in arm64 armhf; do
    MAKE_JOBS="${MAKE_JOBS:-2}" bash amd64/build-vips-debian.sh buster debian:buster true "$arch"
    MAKE_JOBS="${MAKE_JOBS:-2}" bash amd64/build-vips-debian.sh bookworm debian:bookworm false "$arch"
  done
fi

# --- lomo-vips for Ubuntu jammy/noble ---
bash amd64/build-vips-ubuntu.sh jammy ubuntu:22.04
bash amd64/build-vips-ubuntu.sh noble ubuntu:24.04

# --- RPM: Fedora ---
bash rpm/package-fedora.sh

# --- RPM: Rocky Linux 9 ---
build_image_if_missing lomo-vips-rocky9-build rpm/Dockerfile.rocky9-vips-build .
bash rpm/package-vips-generic.sh lomo-vips-rocky9-build rockylinux:9 \
  lomo-vips_8.12.2-2_rocky9_amd64.rpm \
  "glib2-devel, expat-devel, fftw-devel, ImageMagick-devel, orc-devel, lcms2-devel, libheif-devel, zlib-devel, libwebp-devel, libtiff-devel, libpng-devel, libimagequant-devel, libjpeg-turbo-devel, libexif-devel, libgsf-devel, libxml2-devel, openjpeg2-devel, sqlite-devel"
bash rpm/package-rocky9.sh

# --- RPM: openSUSE Leap ---
# Pinned, not :latest: upstream slid that tag from 15.x to 16.0, silently changing which release
# our RPMs target. The same tag is in rpm/Dockerfile.opensuse-vips-build and rpm/package-opensuse.sh,
# and all three must agree -- lomo-vips and lomod have to be built against the same Leap.
build_image_if_missing lomo-vips-opensuse-build rpm/Dockerfile.opensuse-vips-build .
# libheif-aom: Leap 16.0's libheif is 1.23.5, new enough for AVIF, but it ships
# the AV1 decoder as a separate dlopen'd plugin package, so the auto-Requires on
# vips's linked SONAMEs never pulls it and heif_have_decoder_for_format(AV1) comes
# back false -- an .avif stores but gets no preview. Measured, not assumed: Rocky 9's
# libheif 1.23.6 already answers true without any extra package.
bash rpm/package-vips-generic.sh lomo-vips-opensuse-build opensuse/leap:16.0 \
  lomo-vips_8.12.2-2_opensuse_amd64.rpm \
  "sqlite3, libblkid1, util-linux, libheif-aom"
bash rpm/package-opensuse.sh

echo "=== Done. Artifacts: ==="
ls -la "$REPO/releases"
