#!/usr/bin/env bash
# Signs the built lomo-backend RPM with the lomoware apt/rpm key and lays out
# a createrepo_c-generated yum repo under lomoware.github.io/rpm/fedora,
# mirroring the same GitHub-Pages-served layout as the debian/ apt repo.
set -ex

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
RPM_OUT="$REPO/releases"
SITE=/mnt/c/Users/fuji2/Work/lomoware.github.io
RPMREPO="$SITE/rpm/fedora"
GPG_KEY_EMAIL=lomorage@gmail.com

RPM_FILE=$(ls -t "$RPM_OUT"/lomo-backend-*.fc44.x86_64.rpm | head -1)
mkdir -p "$RPMREPO"
cp "$RPM_FILE" "$RPMREPO/"

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
