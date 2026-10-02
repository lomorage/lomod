#!/usr/bin/env bash
# Signs the built lomo-backend + lomo-vips RPMs for Rocky Linux 9 and lays
# out a createrepo_c-generated yum repo under lomoware.github.io/rpm/rocky9.
# Both RPMs go in the same repo dir so dnf can resolve lomo-backend's
# Requires: lomo-vips automatically.
set -ex

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
RPM_OUT="$REPO/releases"
SITE=/mnt/c/Users/fuji2/Work/lomoware.github.io
RPMREPO="$SITE/rpm/rocky9"
GPG_KEY_EMAIL=lomorage@gmail.com

BACKEND_RPM=$(ls -t "$RPM_OUT"/lomo-backend-*.el9.x86_64.rpm | head -1)
VIPS_RPM="$RPM_OUT/lomo-vips_8.12.2_rocky9_amd64.rpm"
mkdir -p "$RPMREPO"
cp "$BACKEND_RPM" "$VIPS_RPM" "$RPMREPO/"

docker run --rm \
  -v "$RPMREPO:/repo" \
  -v "$HOME/.gnupg:/gnupg-ro:ro" \
  fedora:latest bash -c "
    set -ex
    dnf -y install rpm-sign createrepo_c gnupg2 -q
    cp -r /gnupg-ro ~/.gnupg
    chown -R root:root ~/.gnupg
    chmod 700 ~/.gnupg
    cat > ~/.rpmmacros <<EOF
%_signature gpg
%_gpg_name $GPG_KEY_EMAIL
%_gpg_sign_cmd_extra_args --pinentry-mode loopback --batch --yes
EOF
    rpm --addsign /repo/*.rpm
    createrepo_c /repo
    gpg --batch --pinentry-mode loopback --yes --armor --export $GPG_KEY_EMAIL > /repo/repodata/repomd.xml.key
    gpg --batch --pinentry-mode loopback --yes --detach-sign --armor /repo/repodata/repomd.xml
  "

ls -la "$RPMREPO" "$RPMREPO/repodata"
