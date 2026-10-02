#!/usr/bin/env bash
# Packages lomo-backend as an RPM for Fedora. Fedora ships vips directly via
# dnf (8.18.3 at time of writing), and our existing buster-linked lomod/lomoc
# binaries run against it unmodified (glibc forward-compat + Requires: vips
# handles the runtime library, confirmed via smoke test) -- no from-source
# vips build needed here, unlike Ubuntu/Rocky/openSUSE.
set -ex
cd "$(dirname "${BASH_SOURCE[0]}")"
bash package-generic.sh fedora:latest fc \
  "vips, sqlite, zip, perl-Image-ExifTool, ffmpeg, rsync, libblkid, util-linux"
