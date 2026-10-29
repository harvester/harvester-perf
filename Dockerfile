# syntax=docker/dockerfile:1

ARG BCI_GO_TAG=1.26

FROM --platform=$BUILDPLATFORM registry.suse.com/bci/golang:${BCI_GO_TAG} AS base
ARG TARGETOS TARGETARCH
ENV GOOS=${TARGETOS} \
  GOARCH=${TARGETARCH} \
  CGO_ENABLED=0 \
  GOFLAGS=-trimpath

# -------------------------------------------------------------------------------
# etcd benchmark tool is not shipped in the release tarball, so build it from src
# -------------------------------------------------------------------------------
FROM base AS etcd-build
ARG ETCD_VERSION CLI_VERSION
ADD https://github.com/etcd-io/etcd.git#${ETCD_VERSION} /etcd
RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  go build -C /etcd/tools/benchmark -o /out/benchmark \
  && go build -C /etcd/etcdctl -o /out/etcdctl \
  && tar -C /out -cvzf /out/hvperf-tools-"${CLI_VERSION}".tar.gz etcdctl benchmark

FROM scratch AS tools-dist
ARG CLI_VERSION
COPY --from=etcd-build /out/hvperf-tools-"${CLI_VERSION}".tar.gz /

# --------------------
# hvperf target - base
# --------------------
FROM base AS hvperf-base
WORKDIR /go/src

# separate dependencies download to leverage docker layer caching
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# build the source code. use cache mounts to speed up builds by caching go build artifacts and module downloads
COPY . .

# ---------------------
# hvperf target - build
# ---------------------
FROM hvperf-base AS hvperf-build
ARG CLI_VERSION
RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  go build -ldflags "-X github.com/harvester/hvperf/cmd.Version=${CLI_VERSION}" -o /out/hvperf .

# -----------------------------------------
# hvperf target - test
# CGO_ENABLED=1 needed for Go race detector
# -----------------------------------------
FROM hvperf-base AS hvperf-test
ENV CGO_ENABLED=1
RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  go test -cover -race -shuffle=on ./...

# --------------------
# hvperf target - dist
# --------------------
FROM scratch AS hvperf-dist
COPY --from=hvperf-build /out/hvperf /
