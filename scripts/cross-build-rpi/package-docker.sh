#!/usr/bin/env bash
# Packages lomo-backend-docker .debs (installed by the lomorage/lomo-docker
# images) from the lomod .debs package.sh already produced, so the docker
# images ship byte-identical lomod/lomoc binaries to the apt release. Only the
# packaging differs: rpms-build/lomod-docker has no update-lomod timer and its
# maintainer scripts tolerate running without systemd.
#
# Usage: package-docker.sh [version]   (default: releases/LATEST_RELEASE_RPI)
set -ex

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT="$REPO/releases"
VERSION="${1:-$(cat "$OUT/LATEST_RELEASE_RPI")}"
SCHEMA_VERSION="$(ls "$REPO/migrations/sqls/lomod" | grep -cE '^[0-9]+\.go$')"

for arch in armhf arm64 amd64; do
  src="$OUT/lomod_${VERSION}_${arch}.deb"
  [ -f "$src" ] || { echo "missing $src" >&2; exit 1; }

  extracted=$(mktemp -d)
  dist=$(mktemp -d)
  dpkg-deb -x "$src" "$extracted"

  cp -r "$REPO/rpms-build/lomod-docker/"* "$dist/"
  sed -i 's/\r$//' "$dist/DEBIAN/"* "$dist/lib/systemd/system/lomod.service"
  sed -i -e "s/version-replace-me/$VERSION/g" -e "s/arch-replace-me/$arch/g" \
    -e "s/^Package: lomo-backend$/Package: lomo-backend-docker/" "$dist/DEBIAN/control"
  echo "$SCHEMA_VERSION" > "$dist/DEBIAN/schema-version"

  mkdir -p "$dist/opt/lomorage/bin" "$dist/opt/lomorage/var"
  cp "$extracted/opt/lomorage/bin/lomod" "$extracted/opt/lomorage/bin/lomoc" "$dist/opt/lomorage/bin/"
  echo -n "$VERSION" > "$dist/opt/lomorage/LATEST_RELEASE"
  chmod 755 "$dist/opt/lomorage/bin/"* "$dist/DEBIAN/preinst" "$dist/DEBIAN/postinst"
  chmod 644 "$dist/lib/systemd/system/lomod.service"

  # -Zxz: see package.sh (buster/bullseye dpkg can't unpack zstd).
  dpkg-deb --build -Zxz --root-owner-group "$dist" "$OUT/lomo-backend-docker_${VERSION}_${arch}.deb"
  rm -rf "$extracted" "$dist"
done

ls -la "$OUT"/lomo-backend-docker_"${VERSION}"_*.deb
