# Runtime image for integration tests against lomod, published by CI as
# ghcr.io/lomorage/lomod-test. CI builds lomod (on the same Ubuntu release) and
# puts it next to this file before `docker build`.
#
# Besides lomod's own runtime deps it carries what a CI job running *inside*
# the image needs: git for actions/checkout, xz-utils for actions/setup-node.
FROM ubuntu:24.04

# archive.ubuntu.com can crawl from CI (83 kB/s seen, past the job's timeout); the Azure mirror
# GitHub's own runners use is close to them and public, so the image builds the same elsewhere.
RUN sed -i 's|http://archive.ubuntu.com/ubuntu|http://azure.archive.ubuntu.com/ubuntu|g' /etc/apt/sources.list.d/ubuntu.sources  && apt-get update \
 && apt-get install -y --no-install-recommends \
      libvips42t64 libimage-exiftool-perl ffmpeg \
      ca-certificates git xz-utils \
 && rm -rf /var/lib/apt/lists/*

COPY --chmod=755 lomod /opt/lomorage/bin/lomod
ENV PATH=/opt/lomorage/bin:$PATH
