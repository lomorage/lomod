# Runtime image for integration tests against lomod, published by CI as
# ghcr.io/lomorage/lomod-test. CI builds lomod (on the same Ubuntu release) and
# puts it next to this file before `docker build`.
#
# Besides lomod's own runtime deps it carries what a CI job running *inside*
# the image needs: git for actions/checkout, xz-utils for actions/setup-node.
FROM ubuntu:24.04

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      libvips42t64 libimage-exiftool-perl ffmpeg \
      ca-certificates git xz-utils \
 && rm -rf /var/lib/apt/lists/*

COPY --chmod=755 lomod /opt/lomorage/bin/lomod
ENV PATH=/opt/lomorage/bin:$PATH
