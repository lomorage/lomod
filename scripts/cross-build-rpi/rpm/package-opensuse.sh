#!/usr/bin/env bash
# Packages lomo-backend as an RPM for openSUSE Leap. Unlike Fedora, vips
# isn't in openSUSE's base repos, so this Requires our own lomo-vips
# (built by Dockerfile.opensuse-vips-build + package-vips-generic.sh)
# instead of a system vips package.
set -ex
cd "$(dirname "${BASH_SOURCE[0]}")"
bash package-generic.sh opensuse/leap:latest suse \
  "lomo-vips, sqlite3, zip, exiftool, ffmpeg, rsync, libblkid1, util-linux"
