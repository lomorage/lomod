ARG BASE_DOCKER
FROM $BASE_DOCKER

ARG SRC_DIR

COPY vendor $SRC_DIR/vendor
COPY migrations $SRC_DIR/migrations
COPY client $SRC_DIR/client
COPY cmd $SRC_DIR/cmd
COPY common $SRC_DIR/common
COPY handler $SRC_DIR/handler

ARG BUILD_VERSION
RUN cd $SRC_DIR; \
  go install -ldflags "-X bitbucket.org/lomoware/lomo-backend/common/release.Version=$BUILD_VERSION" -v -tags "sqlite_trace trace" ./cmd/lomoc; \
	go install -ldflags "-X bitbucket.org/lomoware/lomo-backend/common/release.Version=$BUILD_VERSION" -v -tags "sqlite_trace trace" ./cmd/lomod
RUN mkdir -p /opt/lomorage/apitest/bin
RUN cd $SRC_DIR; /sbin/ldconfig; \
    /usr/local/bin/staticx /root/bin/lomod /opt/lomorage/apitest/bin/lomod; \
    /usr/local/bin/staticx /root/bin/lomoc /opt/lomorage/apitest/bin/lomoc
