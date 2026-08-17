# syntax=docker/dockerfile:1

ARG BCI_TAG=16.0
ARG BCI_GO_TAG=1.26
ARG ETCD_VERSION=v3.6.14

# -------------------------------------------------------------------------------
# etcd benchmark tool is not shipped in the release tarball, so build it from src
# -------------------------------------------------------------------------------
FROM registry.suse.com/bci/golang:${BCI_GO_TAG} AS etcd-benchmark
ARG ETCD_VERSION
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
RUN git clone --depth 1 --branch "${ETCD_VERSION}" https://github.com/etcd-io/etcd.git /src \
  && cd /src/tools/benchmark \
  && go build -o /out/benchmark

# ---------------------------------------------------------------------------
# build the harvester-perf image. if TARGETARCH is overridden, ensure all the
# *_CHECKSUM values are updated to match the new architecture.
# ---------------------------------------------------------------------------
FROM registry.suse.com/bci/bci-base:${BCI_TAG}
ARG TARGETARCH=amd64
ARG ETCD_VERSION
ARG ETCD_CHECKSUM=sha256:ffe840ff9295808e88cce2794a18a5ac87f12a5203c8314d0bf6aa119b41bac5
ARG JQ_VERSION=1.8.2
ARG JQ_CHECKSUM=sha256:b1c22172dd303f3be49e935aa56aa48a8b7a46e0bc838b4997d3bb451495870f
ARG KUBECTL_VERSION=v1.35.5
ARG KUBECTL_CHECKSUM=sha256:90f75ea6ecc9ea5633262e1c0b83a40560003b30fc94a04cb099404fcef0c224
ARG KUBEVIRT_VERSION=v1.7.1
ARG VIRTCTL_CHECKSUM=sha256:e0efcfc708067fa45232f3bab9cb2de3dbcd812d4c9aab88c727025fb213079f
ARG YQ_VERSION=v4.52.4
ARG YQ_CHECKSUM=sha256:0c4d965ea944b64b8fddaf7f27779ee3034e5693263786506ccd1c120f184e8c

LABEL org.opencontainers.image.title="harvester-perf" \
  org.opencontainers.image.description="Performance and capacity benchmarks for SUSE Harvester" \
  org.opencontainers.image.source="https://github.com/harvester/harvester-perf" \
  io.harvesterhci.image.tools="benchmark:${ETCD_VERSION},etcdctl:${ETCD_VERSION},jq:${JQ_VERSION},kubectl:${KUBECTL_VERSION},virtctl:${KUBEVIRT_VERSION},yq:${YQ_VERSION},"

RUN zypper --non-interactive --gpg-auto-import-keys refresh \
  && zypper --non-interactive install --no-recommends \
  bash coreutils curl tar gzip gawk sed grep diffutils util-linux procps ca-certificates \
  && zypper clean --all

ADD --checksum="${ETCD_CHECKSUM}" "https://github.com/etcd-io/etcd/releases/download/${ETCD_VERSION}/etcd-${ETCD_VERSION}-linux-${TARGETARCH}.tar.gz" /tmp/etcd.tar.gz
ADD --checksum="${JQ_CHECKSUM}" "https://github.com/jqlang/jq/releases/download/jq-${JQ_VERSION}/jq-linux-${TARGETARCH}" /usr/local/bin/jq
ADD --checksum="${KUBECTL_CHECKSUM}" "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" /usr/local/bin/kubectl
ADD --checksum="${VIRTCTL_CHECKSUM}" "https://github.com/kubevirt/kubevirt/releases/download/${KUBEVIRT_VERSION}/virtctl-${KUBEVIRT_VERSION}-linux-${TARGETARCH}" /usr/local/bin/virtctl
ADD --checksum="${YQ_CHECKSUM}" "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_${TARGETARCH}" /usr/local/bin/yq
COPY --from=etcd-benchmark /out/benchmark /usr/local/bin/benchmark

RUN set -eux; \
  tar -xzf /tmp/etcd.tar.gz -C /tmp; \
  mv "/tmp/etcd-${ETCD_VERSION}-linux-${TARGETARCH}/etcdctl" /usr/local/bin/etcdctl; \
  rm -rf /tmp/etcd.tar.gz "/tmp/etcd-${ETCD_VERSION}-linux-${TARGETARCH}"; \
  chmod +x /usr/local/bin/kubectl /usr/local/bin/jq /usr/local/bin/yq /usr/local/bin/etcdctl; \
  kubectl version --client=true --output=yaml; \
  jq --version; \
  yq --version; \
  etcdctl version; \
  benchmark --help

ENV PERF_HOME=/opt/harvester-perf \
  RESULTS_DIR=/results \
  KUBECONFIG=/root/.kube/config \
  ETCDCTL_API=3

COPY entrypoint.sh ${PERF_HOME}/entrypoint.sh
COPY lib/    ${PERF_HOME}/lib/
COPY suites/ ${PERF_HOME}/suites/

RUN chmod +x ${PERF_HOME}/entrypoint.sh ${PERF_HOME}/suites/*.sh \
  && ln -s ${PERF_HOME}/entrypoint.sh /usr/local/bin/harvester-perf \
  && mkdir -p ${RESULTS_DIR}

WORKDIR /opt/harvester-perf
ENTRYPOINT ["./entrypoint.sh"]
CMD ["run"]
