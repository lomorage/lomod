ARG BASE_DOCKER
FROM $BASE_DOCKER

ARG SRC_DIR

COPY vendor $SRC_DIR/vendor
COPY cmd $SRC_DIR/cmd
COPY common $SRC_DIR/common
COPY api $SRC_DIR/api

RUN cd $SRC_DIR/api/test/utils/gen-info; go build --tags 'sqlite_trace trace'

WORKDIR $SRC_DIR/api/test/utils/gen-info
