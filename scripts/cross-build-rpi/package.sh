#!/usr/bin/env bash
# Packages the binaries built by build.sh (in /tmp) into .deb files, using
# the packaging metadata in rpms-build/lomod. No sudo/Docker needed.
set -ex

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SUBMOD="$REPO/rpms-build/lomod/rpms-build"

if [ ! -f "$SUBMOD/release/DEBIAN/postinst" ]; then
  echo "rpms-build/lomod packaging files missing under $SUBMOD" >&2
  exit 1
fi

OUT="$REPO/releases"
mkdir -p "$OUT"

VERSION="$(date +%Y-%m-%d.%H-%M-%S).0.$(cd "$REPO" && git rev-parse --short=7 HEAD)"

# Number of DB migration statements this build ships (migrations/sqls/lomod/N.go,
# 0-indexed). preinst compares this against the live DB's applied count to
# decide whether an upgrade will actually touch the schema, so it can skip
# backing up assets.db on routine no-op upgrades.
SCHEMA_VERSION="$(ls "$REPO/migrations/sqls/lomod" | grep -cE '^[0-9]+\.go$')"

build_one() {
  local binsuffix="$1" controlfile="$2" outname="$3"
  local dist
  dist=$(mktemp -d)

  mkdir -p "$dist/opt/lomorage/bin" "$dist/opt/lomorage/var"
  cp -r "$SUBMOD/release/"* "$dist/"
  mkdir -p "$dist/DEBIAN"
  cp "$SUBMOD/$controlfile" "$dist/DEBIAN/control"
  sed -i "s/version-replace-me/$VERSION/g" "$dist/DEBIAN/control"
  echo "$SCHEMA_VERSION" > "$dist/DEBIAN/schema-version"

  # rpms-build/lomod can end up with CRLF line endings on Windows (git
  # autocrlf, if .gitattributes' eol=lf is bypassed), which breaks the shebang on these maintainer/update
  # scripts (dpkg tries to exec "/bin/bash\r" and fails with ENOENT) and makes
  # systemd reject unit file lines. Strip any stray \r regardless of how the
  # checkout ended up.
  sed -i 's/\r$//' "$dist/DEBIAN/preinst" "$dist/DEBIAN/postinst" "$dist/opt/lomorage/bin/update-lomod" \
    "$dist"/lib/systemd/system/lomod.service "$dist"/lib/systemd/system/lomod-update.service "$dist"/lib/systemd/system/lomod-update.timer

  cp "/tmp/lomod-$binsuffix" "$dist/opt/lomorage/bin/lomod"
  cp "/tmp/lomoc-$binsuffix" "$dist/opt/lomorage/bin/lomoc"
  chmod 755 "$dist/opt/lomorage/bin/lomod" "$dist/opt/lomorage/bin/lomoc" "$dist/opt/lomorage/bin/update-lomod"
  chmod 644 "$dist"/lib/systemd/system/lomod*
  chmod 755 "$dist/DEBIAN/postinst" "$dist/DEBIAN/preinst"

  # -Zxz: modern dpkg-deb defaults to zstd, which buster/bullseye's dpkg
  # (1.19/1.20) can't unpack at all ("unknown compression for member
  # control.tar.zst") -- xz has been supported since ancient dpkg versions.
  dpkg-deb --build -Zxz --root-owner-group "$dist" "$OUT/$outname"
  rm -rf "$dist"
}

# amd64 binaries come from scripts/cross-build-rpi/amd64/build.sh instead of
# /tmp (they're built natively inside a Docker container, not via build.sh's
# cross-compile path); stage them under /tmp to match build_one's convention.
if [ -f "$REPO/scripts/cross-build-rpi/amd64/out/lomod-amd64" ]; then
  cp "$REPO/scripts/cross-build-rpi/amd64/out/lomod-amd64" /tmp/lomod-amd64
  cp "$REPO/scripts/cross-build-rpi/amd64/out/lomoc-amd64" /tmp/lomoc-amd64
fi

build_one armhf control_pi       "lomod_${VERSION}_armhf.deb"
build_one arm64 control_pi_arm64 "lomod_${VERSION}_arm64.deb"
[ -f /tmp/lomod-amd64 ] && build_one amd64 control_x86 "lomod_${VERSION}_amd64.deb"

echo "$VERSION" > "$OUT/LATEST_RELEASE_RPI"
ls -la "$OUT"
