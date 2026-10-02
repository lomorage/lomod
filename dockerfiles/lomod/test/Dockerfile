ARG BASE_DOCKER
FROM $BASE_DOCKER

# TODO: remove when new base image is updated
RUN apt update && apt install -y kmod zbar-tools

RUN apt update && apt install -y vim

ARG SRC_DIR

RUN mkdir -p $SRC_DIR/cmd/lomod/
COPY cmd/lomod/test $SRC_DIR/cmd/lomod/test
COPY vendor $SRC_DIR/vendor
COPY migrations $SRC_DIR/migrations
COPY client $SRC_DIR/client
COPY api $SRC_DIR/api
COPY common $SRC_DIR/common
COPY handler $SRC_DIR/handler
COPY Makefile go.mod go.sum $SRC_DIR/

RUN mkdir -p /opt/lomorage/bin
COPY artifacts/opt/lomorage/bin/lomod /opt/lomorage/bin

RUN /usr/sbin/useradd -m -d $SRC_DIR pi
RUN echo "pi ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/010_pi-nopasswd
ENV PATH /usr/bin:/bin:/usr/sbin:/sbin:/usr/local/bin:/usr/local/go/bin
WORKDIR $SRC_DIR/
