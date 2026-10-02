#!/usr/bin/env bash
# Builds lomod+lomoc for amd64 natively inside the buster build image
# (see Dockerfile.buster-build) -- no cross-compilation needed since the
# WSL host is already amd64. Requires the image to already be built:
#   docker build -t lomod-amd64-buster-build -f Dockerfile.buster-build .
set -ex

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
mkdir -p "$REPO/scripts/cross-build-rpi/amd64/out"

# Without this, the binary panics on startup with `could not locate box
# "../cmd/lomod/static"`. Generated on the host (not inside the container)
# since the repo is bind-mounted -- the container sees it either way.
# Matches what `make build-lomod` does via its build-rice target. Reuses
# the same Go toolchain location build.sh (arm64/armhf) sets up.
GO_VERSION=1.26.5
GO_TOOLCHAIN="$HOME/go-toolchain"
if [ ! -x "$GO_TOOLCHAIN/bin/go" ]; then
  tmp=$(mktemp -d)
  curl -fsSL -o "$tmp/go.tar.gz" "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
  mkdir -p "$GO_TOOLCHAIN"
  tar -C "$GO_TOOLCHAIN" --strip-components=1 -xzf "$tmp/go.tar.gz"
  rm -rf "$tmp"
fi
export PATH="$GO_TOOLCHAIN/bin:$HOME/go/bin:$PATH"
command -v rice >/dev/null || go install github.com/GeertJohan/go.rice/rice@v1.0.2
( cd "$REPO/handler" && rice embed-go )

docker run --rm \
  -v "$REPO:/src" -w /src \
  -e CGO_ENABLED=1 \
  -e GOOS=linux \
  -e GOARCH=amd64 \
  -e CGO_CFLAGS_ALLOW=-Xpreprocessor \
  -e GOCACHE=/tmp/gocache \
  -e GOPATH=/tmp/gopath \
  lomod-amd64-buster-build \
  bash -c '
    set -ex
    go build -mod=vendor -buildvcs=false -tags "sqlite_trace trace netgo" -ldflags "-s -w" -o /src/scripts/cross-build-rpi/amd64/out/lomod-amd64 ./cmd/lomod
    go build -mod=vendor -buildvcs=false -tags "sqlite_trace trace netgo" -ldflags "-s -w" -o /src/scripts/cross-build-rpi/amd64/out/lomoc-amd64 ./cmd/lomoc
  '

file "$REPO/scripts/cross-build-rpi/amd64/out/lomod-amd64" "$REPO/scripts/cross-build-rpi/amd64/out/lomoc-amd64"
echo "max GLIBC required:"
objdump -T "$REPO/scripts/cross-build-rpi/amd64/out/lomod-amd64" 2>/dev/null | grep -oE 'GLIBC_[0-9.]+' | sort -Vu | tail -1
