#!/usr/bin/env bash
# One-time (repeat only if ~/sysroots gets wiped): pulls the lomorage buster
# build images and extracts /usr + /lib into sysroots usable by a native
# amd64 cross-gcc via --sysroot. Needs docker (see 00-setup-docker.sh).
set -ex

mkdir -p "$HOME/sysroots/aarch64" "$HOME/sysroots/armhf"

extract() {
  local arch="$1" img="$2" platform="$3"
  docker pull --platform "$platform" "$img"
  local cid
  cid=$(docker create --platform "$platform" "$img")
  docker cp "$cid:/usr" "$HOME/sysroots/$arch/usr"
  docker cp "$cid:/lib" "$HOME/sysroots/$arch/lib" || true
  docker rm "$cid" >/dev/null
  echo "extracted $arch"
}

extract aarch64 lomorage/vips-aarch64-debian-buster-build:latest linux/arm64
extract armhf   lomorage/vips-armv7hf-debian-buster-build:latest linux/arm/v7

# Absolute symlinks (e.g. libresolv.so -> /lib/aarch64-linux-gnu/libresolv.so.2)
# escape --sysroot at the kernel level. Rewrite them relative so they resolve
# correctly no matter what root they're followed from. See README.md.
fix_symlinks() {
  local sysroot="$1"
  local fixed=0 unresolved=0
  while IFS= read -r -d '' link; do
    target=$(readlink "$link")
    case "$target" in
      /*)
        newtarget="$sysroot$target"
        if [ -e "$newtarget" ]; then
          ln -sfr "$newtarget" "$link"
          fixed=$((fixed+1))
        else
          unresolved=$((unresolved+1))
        fi
        ;;
    esac
  done < <(find "$sysroot" -type l -print0)
  echo "$sysroot: fixed=$fixed unresolved=$unresolved (unresolved links point outside /usr,/lib and are expected/unused)"
}

fix_symlinks "$HOME/sysroots/aarch64"
fix_symlinks "$HOME/sysroots/armhf"

du -sh "$HOME/sysroots/aarch64" "$HOME/sysroots/armhf"
