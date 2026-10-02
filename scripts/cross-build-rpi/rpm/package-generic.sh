#!/usr/bin/env bash
# Shared RPM packaging logic used by package-fedora.sh / package-rocky9.sh /
# package-opensuse.sh. Not meant to be called directly.
# Usage: package-generic.sh <build-image> <dist-tag> <requires-line>
set -ex

BUILD_IMAGE="$1"
DIST_TAG="$2"
REQUIRES="$3"

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SUBMOD="$REPO/rpms-build/lomod/rpms-build"
AMD64_OUT="$REPO/scripts/cross-build-rpi/amd64/out"

if [ ! -f "$SUBMOD/release/DEBIAN/postinst" ]; then
  echo "rpms-build/lomod packaging files missing under $SUBMOD" >&2
  exit 1
fi
if [ ! -f "$AMD64_OUT/lomod-amd64" ]; then
  echo "amd64 binaries not built; run scripts/cross-build-rpi/amd64/build.sh first" >&2
  exit 1
fi

OUT="$REPO/releases"
mkdir -p "$OUT"

VERSION="$(date +%Y%m%d.%H%M%S)"
# Number of DB migration statements this build ships (migrations/sqls/lomod/N.go,
# 0-indexed); baked into %pre's assets.db-backup-skip check, see spec.in.
SCHEMA_VERSION="$(ls "$REPO/migrations/sqls/lomod" | grep -cE '^[0-9]+\.go$')"
BUILDROOT=$(mktemp -d)

mkdir -p "$BUILDROOT/opt/lomorage/bin" "$BUILDROOT/opt/lomorage/var" "$BUILDROOT/opt/lomorage/etc" "$BUILDROOT/opt/lomorage/doc"
mkdir -p "$BUILDROOT/lib/systemd/system"

cp "$SUBMOD/release/opt/lomorage/var/world.db" "$BUILDROOT/opt/lomorage/var/"
cp "$SUBMOD/release/opt/lomorage/etc/environment" "$BUILDROOT/opt/lomorage/etc/"
cp "$SUBMOD/release/opt/lomorage/doc/readme.txt" "$SUBMOD/release/opt/lomorage/doc/"*.txt "$BUILDROOT/opt/lomorage/doc/"
cp "$SUBMOD"/release/lib/systemd/system/lomod.service "$SUBMOD"/release/lib/systemd/system/lomod-update.service \
  "$SUBMOD"/release/lib/systemd/system/lomod-update.timer "$BUILDROOT/lib/systemd/system/"
cp "$SUBMOD/release/opt/lomorage/bin/update-lomod" "$BUILDROOT/opt/lomorage/bin/"
# same CRLF issue as the .deb packaging (see package.sh) -- these come from
# the same rpms-build/lomod files.
sed -i 's/\r$//' "$BUILDROOT/opt/lomorage/bin/update-lomod" "$BUILDROOT"/lib/systemd/system/lomod*
chmod 755 "$BUILDROOT/opt/lomorage/bin/update-lomod"
chmod 644 "$BUILDROOT"/lib/systemd/system/lomod*

cp "$AMD64_OUT/lomod-amd64" "$BUILDROOT/opt/lomorage/bin/lomod"
cp "$AMD64_OUT/lomoc-amd64" "$BUILDROOT/opt/lomorage/bin/lomoc"
chmod 755 "$BUILDROOT/opt/lomorage/bin/lomod" "$BUILDROOT/opt/lomorage/bin/lomoc"

sed "s/%{_lomo_version}/$VERSION/; s|%{_lomo_buildroot}|/work/buildroot|; s|%{_lomo_requires}|$REQUIRES|; s/%{_lomo_schema_version}/$SCHEMA_VERSION/" \
  "$REPO/scripts/cross-build-rpi/rpm/lomo-backend.spec.in" > "$BUILDROOT.spec"

docker run --rm \
  -v "$BUILDROOT:/work/buildroot:ro" \
  -v "$BUILDROOT.spec:/work/lomo-backend.spec:ro" \
  -v "$OUT:/work/out" \
  "$BUILD_IMAGE" bash -c '
    set -ex
    if command -v dnf >/dev/null; then dnf -y install rpm-build -q; else zypper --non-interactive install -y rpm-build; fi
    rpmbuild --define "_topdir /tmp/rpmbuild" -bb /work/lomo-backend.spec
    cp /tmp/rpmbuild/RPMS/x86_64/*.rpm /work/out/
  '

rm -rf "$BUILDROOT" "$BUILDROOT.spec"
ls -la "$OUT"/lomo-backend-"$VERSION"*.rpm
