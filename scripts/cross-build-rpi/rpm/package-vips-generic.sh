#!/usr/bin/env bash
# Packages the /usr/local content of a vips-build image (produced by
# Dockerfile.rocky9-vips-build or Dockerfile.opensuse-vips-build) into a
# lomo-vips RPM. Not meant to be called directly.
# Usage: package-vips-generic.sh <build-image> <rpm-build-image> <outname> <requires-line>
set -ex

VIPS_IMAGE="$1"
RPM_BUILD_IMAGE="$2"
OUTNAME="$3"
REQUIRES="$4"
VIPS_VERSION="${VIPS_VERSION:-8.12.2}"

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
OUT="$REPO/releases"
mkdir -p "$OUT"

BUILDROOT=$(mktemp -d)
mkdir -p "$BUILDROOT/usr/local" "$BUILDROOT/etc/ld.so.conf.d"

cid=$(docker create "$VIPS_IMAGE")
docker cp "$cid:/usr/local/." "$BUILDROOT/usr/local/"
docker rm "$cid" >/dev/null

# RHEL/Rocky's default ld search path doesn't include /usr/local/lib
# (openSUSE's does, but this is harmless there too).
echo "/usr/local/lib" > "$BUILDROOT/etc/ld.so.conf.d/lomo-vips.conf"

# RHEL/Rocky's rpmbuild rejects ambiguous "#!/usr/bin/python" shebangs
# (vips bundles a couple of profiling/debug scripts using it).
grep -rlZ '^#!/usr/bin/python$' "$BUILDROOT/usr/local/bin" 2>/dev/null | \
  xargs -0 -r sed -i '1s|^#!/usr/bin/python$|#!/usr/bin/python3|'

sed "s/%{_lomo_vips_version}/$VIPS_VERSION/; s|%{_lomo_buildroot}|/work/buildroot|; s|%{_lomo_requires}|$REQUIRES|" \
  "$REPO/scripts/cross-build-rpi/rpm/lomo-vips.spec.in" > "$BUILDROOT.spec"

docker run --rm \
  -v "$BUILDROOT:/work/buildroot:ro" \
  -v "$BUILDROOT.spec:/work/lomo-vips.spec:ro" \
  -v "$OUT:/work/out" \
  "$RPM_BUILD_IMAGE" bash -c '
    set -ex
    if command -v dnf >/dev/null; then dnf -y install rpm-build -q; else zypper --non-interactive install -y rpm-build; fi
    rpmbuild --define "_topdir /tmp/rpmbuild" -bb /work/lomo-vips.spec
    cp /tmp/rpmbuild/RPMS/x86_64/*.rpm "/work/out/$0"
  ' "$OUTNAME"

rm -rf "$BUILDROOT" "$BUILDROOT.spec"
ls -la "$OUT/$OUTNAME"
