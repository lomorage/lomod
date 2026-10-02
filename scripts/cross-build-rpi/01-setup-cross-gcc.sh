#!/usr/bin/env bash
# One-time: native amd64 cross-compilers targeting arm64/armhf, plus pkg-config.
# Run manually in a WSL2 Ubuntu terminal -- needs your sudo password interactively.
set -ex

sudo apt-get update
sudo apt-get install -y \
  gcc-aarch64-linux-gnu binutils-aarch64-linux-gnu \
  gcc-arm-linux-gnueabihf binutils-arm-linux-gnueabihf \
  pkg-config

aarch64-linux-gnu-gcc --version | head -1
arm-linux-gnueabihf-gcc --version | head -1
