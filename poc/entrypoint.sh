#!/usr/bin/env bash
# harvester-perf entrypoint: run one or more benchmark suites and emit a report.
set -euo pipefail

export PERF_HOME="${PERF_HOME:-/opt/harvester-perf}"
# shellcheck source=lib/common.sh
. "$PERF_HOME/lib/common.sh"
# shellcheck source=lib/k8s.sh
. "$PERF_HOME/lib/k8s.sh"

# the default suites are made up of "read-only" tests that do not create or
# delete workloads, so they can be run on a live cluster without disruption.
DEFAULT_SUITES="cluster-info node-capacity etcd-benchmark controlplane-resources"
ALL_SUITES="${DEFAULT_SUITES}"

usage() {
  cat <<EOF
harvester-perf - performance and capacity benchmarks for SUSE Harvester

Usage:
  harvester-perf run [SUITE...]   Run suites (default: $DEFAULT_SUITES)
  harvester-perf run all          Run all suite defined in $ALL_SUITES
  harvester-perf list             List available suites
  harvester-perf report DIR       Re-render report.json/summary.md from a run directory

Suites:
  cluster-info            Versions and topology of the cluster under test
  node-capacity           CPU / memory / storage capacity and usage per host
  etcd-benchmark          Upstream etcd benchmark + disk latency metrics
  controlplane-resources  Requests, limits and live usage of control plane pods

Common environment variables:
  KUBECONFIG=/root/.kube/config   Kubeconfig to use
  RESULTS_DIR=/results            Where run directories are written
  FAIL_FAST=true                  Abort the run on the first failing suite
  NO_COLOR=1                      Disable ANSI colour
  PERF_UID / PERF_GID             chown results to this uid/gid on exit

  CP_NAMESPACES="kube-system harvester-system longhorn-system"
  ETCD_SMALL_TOTAL / ETCD_LARGE_TOTAL / ETCD_LARGE_CONNS / ETCD_LARGE_CLIENTS
  ETCD_CHECK_PERF=true            Also run 'etcdctl check perf' (~60s)
  ETCD_BENCH_IMAGE                Image for the in-cluster etcd helper pod
EOF
}

preflight() {
  [ -r "${KUBECONFIG:-$HOME/.kube/config}" ] ||
    die "no readable kubeconfig at ${KUBECONFIG:-$HOME/.kube/config} - mount one with -v ~/.kube/config:/root/.kube/config:ro"
  local server
  server=$(api_server)
  case "$server" in
  *127.0.0.1* | *localhost*)
    warn "kubeconfig points at $server, which resolves to the container itself."
    warn "Re-run with 'docker run --network host' or point the kubeconfig at a routable address."
    ;;
  esac
  require_cluster
}

cmd_run() {
  local suites="$*"
  if [ -z "$suites" ]; then
    suites="$DEFAULT_SUITES"
  elif [ "$suites" = "all" ]; then
    suites="$ALL_SUITES"
  fi

  for s in $suites; do
    [ -f "$PERF_HOME/suites/$s.sh" ] || die "unknown suite: $s (see 'harvester-perf list')"
  done

  export RUN_ID="$(date -u +%Y%m%d-%H%M%S)"
  export RUN_DIR="${RUN_DIR:-$RESULTS_DIR/$RUN_ID}"
  mkdir -p "$RUN_DIR" || die "cannot write to $RUN_DIR - mount a results volume with -v \$PWD/results:/results"
  export SUITE_ORDER="$suites"

  printf '%s\n' "${C_BOLD}harvester-perf${C_RESET} run ${C_BOLD}$RUN_ID${C_RESET}"
  preflight
  info "results: $RUN_DIR"
  info "suites : $suites"

  # ensures empty status file
  : >"$RUN_DIR/.status"

  # run the selected test suites
  local failed=0
  for s in $suites; do
    local start_timestamp
    local end_timestamp
    local rc=0
    start_timestamp=$(date +%s)
    if "$PERF_HOME/suites/$s.sh" 2>&1 | tee -a "$RUN_DIR/run.log"; then
      rc=0
    else
      rc=${PIPESTATUS[0]}
    fi
    end_timestamp=$(date +%s)

    if [ "$rc" -eq 0 ]; then
      printf '%s\tpassed\t%s\n' "$s" "$((end_timestamp - start_timestamp))" >>"$RUN_DIR/.status"
    else
      printf '%s\tFAILED\t%s\n' "$s" "$((end_timestamp - start_timestamp))" >>"$RUN_DIR/.status"
      err "suite $s failed (exit $rc)"
      failed=$((failed + 1))
      if [ "${FAIL_FAST:-false}" = "true" ]; then
        break
      fi
    fi
  done

  # produce the test report
  "$PERF_HOME/lib/report.sh" | tee -a "$RUN_DIR/run.log"
  chown_report
  [ "$failed" -eq 0 ] || exit 1
}

chown_report() {
  if [ -n "${PERF_UID:-}" ]; then
    chown -R "${PERF_UID}:${PERF_GID:-$PERF_UID}" "$RUN_DIR" 2>/dev/null || true
  fi
}

case "${1:-run}" in
-h | --help | help) usage ;;
list) printf '%s\n' $ALL_SUITES ;;
run)
  shift || true
  cmd_run "$@"
  ;;
report)
  shift || true
  export RUN_DIR="${1:?usage: harvester-perf report <run-dir>}"
  export RUN_ID="${RUN_ID:-$(basename "$RUN_DIR")}"
  bash "$PERF_HOME/bin/report.sh"
  ;;
*)
  usage
  exit 2
  ;;
esac
