#!/usr/bin/env bash
# Packages lomo-backend as an RPM for Rocky Linux 9 (and RHEL/Alma 9). Like
# openSUSE, vips isn't in the base repos at a usable version, so this
# Requires our own lomo-vips (built by Dockerfile.rocky9-vips-build +
# package-vips-generic.sh) instead of a system vips package.
# perl-Image-ExifTool needs EPEL and ffmpeg needs RPM Fusion on the target
# system (documented in lomodoc as prerequisites, same pattern as Fedora).
set -ex
cd "$(dirname "${BASH_SOURCE[0]}")"
bash package-generic.sh rockylinux:9 el9 \
  "lomo-vips, sqlite, zip, perl-Image-ExifTool, ffmpeg, rsync, libblkid, util-linux"
