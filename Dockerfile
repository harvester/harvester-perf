# syntax=docker/dockerfile:1

ARG BCI_TAG=16.0
ARG BCI_GO_TAG=1.26
ARG ETCD_VERSION=v3.6.14

# -------------------------------------------------------------------------------
# etcd benchmark tool is not shipped in the release tarball, so build it from src
# -------------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM registry.suse.com/bci/golang:${BCI_GO_TAG} AS etcd-builder
ARG ETCD_VERSION \
  TARGETOS \
  TARGETARCH
ENV GOOS=${TARGETOS} \
  GOARCH=${TARGETARCH} \
  CGO_ENABLED=0 \
  GOFLAGS=-trimpath
RUN git clone --depth 1 --branch "${ETCD_VERSION}" https://github.com/etcd-io/etcd.git /src \
  && go build -C /src/tools/benchmark -o /out/benchmark \
  && go build -C /src/etcdctl -o /out/etcdctl

# -------------------------------------------------------------------------------
# build the harvester-perf binary
# -------------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM registry.suse.com/bci/golang:${BCI_GO_TAG} AS hvperf-builder
ARG TARGETOS \
  TARGETARCH
ENV GOOS=${TARGETOS} \
  GOARCH=${TARGETARCH} \
  CGO_ENABLED=0 \
  GOFLAGS=-trimpath
WORKDIR /go/src

# separate dependencies download to leverage docker layer caching
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# build the source code. use cache mounts to speed up builds by caching go build artifacts and module downloads
COPY . .
ARG CLI_VERSION
RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  go build -ldflags "-X github.com/harvester/hvperf/cmd.Version=${CLI_VERSION}" -o /out/hvperf .

# ---------------------------------------------------------------------------
# build the harvester-perf image. this image includes all the performance and
# benchmarking tools needed to run the tests
# ---------------------------------------------------------------------------
FROM registry.suse.com/bci/bci-base:${BCI_TAG}
ARG ETCD_VERSION
LABEL org.opencontainers.image.title="harvester-perf" \
  org.opencontainers.image.description="Performance and capacity benchmarks for SUSE Harvester" \
  org.opencontainers.image.source="https://github.com/harvester/harvester-perf" \
  io.harvesterhci.image.tools="benchmark:${ETCD_VERSION},etcdctl:${ETCD_VERSION}"

RUN zypper --non-interactive --gpg-auto-import-keys refresh \
  && zypper --non-interactive install --no-recommends \
  bash coreutils curl tar gzip gawk sed grep diffutils util-linux procps ca-certificates \
  && zypper clean --all

COPY --from=etcd-builder /out/benchmark /usr/local/bin/benchmark
COPY --from=etcd-builder /out/etcdctl /usr/local/bin/etcdctl
COPY --from=hvperf-builder /out/hvperf /usr/local/bin/hvperf

RUN set -eux ;\
  etcdctl version; \
  benchmark --help
ENV KUBECONFIG=/root/.kube/config \
  ETCDCTL_API=3
ENTRYPOINT ["/usr/local/bin/hvperf"]
CMD ["version"]
