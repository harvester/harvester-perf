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
  && go build -C /etcd/etcdctl -o /out/etcdctl

# -------------------------
# build netperf from source
# -------------------------
FROM base AS netperf-builder
ARG NETPERF_VERSION
RUN git clone https://github.com/HewlettPackard/netperf /netperf
WORKDIR /netperf
RUN git reset --hard ${NETPERF_VERSION} \
  && zypper install -y autoconf automake libtool  \
  &&./autogen.sh  \
  && ./configure --enable-sctp=yes --enable-demo=yes MAKEINFO=true \
  && make \
  && make install \
  && mkdir /out \
  && cp /usr/local/bin/netperf /out/netperf

# ------------------------
# build iperf3 from source
# ------------------------
FROM base AS iperf-builder
ARG IPERF3_VERSION
RUN curl -L https://github.com/esnet/iperf/releases/download/${IPERF3_VERSION}/iperf-${IPERF3_VERSION}.tar.gz | tar xz \
  && mv iperf-${IPERF3_VERSION} /iperf
WORKDIR /iperf
RUN ./configure; make install \
  && mkdir /out \
  && cp /usr/local/bin/iperf3 /out/iperf3 \
  && cp /usr/local/lib/libiperf.so* /out/

# ---------------------------------------------------------------------
# bundle all the benchmark tools into a single tarball for distribution
# ---------------------------------------------------------------------
FROM base AS tools-build
ARG CLI_VERSION
COPY --from=etcd-build /out/ /out/
COPY --from=netperf-builder /out/netperf /out/
COPY --from=iperf-builder /out/* /out/
RUN tar -C /out -cvzf /hvperf-tools-${CLI_VERSION}.tar.gz .

# ---------------------------------------------------------------------
# distribute the benchmark tools tarball; expect to be used with buildkit
# --output type=local,dest=.
# ---------------------------------------------------------------------
FROM scratch AS tools-dist
ARG CLI_VERSION
COPY --from=tools-build /hvperf-tools-${CLI_VERSION}.tar.gz /

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
