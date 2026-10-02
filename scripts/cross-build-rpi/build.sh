#!/usr/bin/env bash
# Cross-builds lomod + lomoc for Raspberry Pi against the Debian buster
# sysroots extracted by 02-extract-sysroots.sh. Usage:
#   build.sh arm64    (aarch64, 64-bit Pi OS)
#   build.sh armhf    (armv7, 32-bit Pi OS)
set -ex

TARGET="$1"
case "$TARGET" in
  arm64)
    SYSROOT="$HOME/sysroots/aarch64"
    CC=aarch64-linux-gnu-gcc
    GOARCH=arm64
    GOARM=
    LIBDIR=aarch64-linux-gnu
    EXTRA_CFLAGS=
    ;;
  armhf)
    SYSROOT="$HOME/sysroots/armhf"
    CC=arm-linux-gnueabihf-gcc
    GOARCH=arm
    GOARM=7
    LIBDIR=arm-linux-gnueabihf
    # Ubuntu 24.04's 32-bit ARM cross-gcc defaults to the 64-bit time_t ABI,
    # which buster's glibc doesn't have. Force the classic 32-bit API.
    EXTRA_CFLAGS=-D_TIME_BITS=32
    ;;
  *)
    echo "usage: $0 {arm64|armhf}" >&2
    exit 1
    ;;
esac

GO_VERSION=1.26.5
GO_TOOLCHAIN="$HOME/go-toolchain"
if [ ! -x "$GO_TOOLCHAIN/bin/go" ]; then
  tmp=$(mktemp -d)
  curl -fsSL -o "$tmp/go.tar.gz" "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
  mkdir -p "$GO_TOOLCHAIN"
  tar -C "$GO_TOOLCHAIN" --strip-components=1 -xzf "$tmp/go.tar.gz"
  rm -rf "$tmp"
fi

export PATH="$GO_TOOLCHAIN/bin:$PATH"
export CC
export CGO_ENABLED=1
export GOOS=linux
export GOARCH
export GOARM
export CGO_CFLAGS_ALLOW=-Xpreprocessor
export CGO_CFLAGS="--sysroot=$SYSROOT $EXTRA_CFLAGS"
# -static-libgcc: avoid pulling in the host cross-toolchain's own libgcc_s.so.1
# (built for a much newer glibc than buster's). See README.md.
export CGO_LDFLAGS="--sysroot=$SYSROOT -static-libgcc"
export PKG_CONFIG_LIBDIR="$SYSROOT/usr/lib/$LIBDIR/pkgconfig:$SYSROOT/usr/local/lib/pkgconfig:$SYSROOT/usr/share/pkgconfig"
export PKG_CONFIG_SYSROOT_DIR="$SYSROOT"

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO"

pkg-config --cflags --libs vips sqlite3 >/dev/null

# Without this, the binary panics on startup with `could not locate box
# "../cmd/lomod/static"` -- rice.MustFindBox() needs either this generated
# file (embedding the static UI assets into the binary) or the actual
# static/ directory to exist next to the running binary, which it won't in
# a packaged install. Matches what `make build-lomod` does via its
# build-rice target.
if ! command -v rice >/dev/null; then
  # unset GOARCH/GOOS/CC/CGO_* for this one install: they're set above for
  # cross-compiling lomod/lomoc, but the rice tool itself must run natively
  # on this (amd64) host, and `go install` refuses to put a cross-compiled
  # binary in GOBIN.
  env -u GOARCH -u GOARM -u CC -u CGO_ENABLED -u CGO_CFLAGS -u CGO_LDFLAGS \
    GOOS=linux GOARCH=amd64 GOBIN="$GO_TOOLCHAIN/bin" \
    go install github.com/GeertJohan/go.rice/rice@v1.0.2
fi
( cd handler && rice embed-go )

# netgo: this sysroot's libresolv only exports __res_search, not the plain
# res_search alias Go's cgo resolver expects. netgo uses the pure-Go
# resolver instead, sidestepping that symbol. CGO stays on for sqlite3/vips.
BUILD_TAGS='sqlite_trace trace netgo'

go build -mod=vendor -tags "$BUILD_TAGS" -ldflags "-s -w" -o "/tmp/lomod-$TARGET" ./cmd/lomod
go build -mod=vendor -tags "$BUILD_TAGS" -ldflags "-s -w" -o "/tmp/lomoc-$TARGET" ./cmd/lomoc

file "/tmp/lomod-$TARGET" "/tmp/lomoc-$TARGET"
echo "max GLIBC required:"
objdump -T "/tmp/lomod-$TARGET" 2>/dev/null | grep -oE 'GLIBC_[0-9.]+' | sort -Vu | tail -1
