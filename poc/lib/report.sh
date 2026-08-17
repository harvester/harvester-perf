#!/usr/bin/env bash
# Merge the per-suite artifacts in $RUN_DIR into report.json + summary.md.
set -euo pipefail

PERF_HOME="${PERF_HOME:-/opt/harvester-perf}"
# shellcheck source=../lib/common.sh
. "$PERF_HOME/lib/common.sh"
# shellcheck source=../lib/k8s.sh
. "$PERF_HOME/lib/k8s.sh"

: "${RUN_DIR:?RUN_DIR must be set}"

SUITE_ORDER="${SUITE_ORDER:-cluster-info node-capacity etcd-benchmark controlplane-resources}"
STATUS_FILE="$RUN_DIR/.status"
[ -f "$STATUS_FILE" ] || : >"$STATUS_FILE"

# --------------------------------------------------------------- report.json --
merge_args=()
for suite in $SUITE_ORDER; do
  [ -f "$RUN_DIR/$suite.json" ] || continue
  merge_args+=(--slurpfile "s_${suite//-/_}" "$RUN_DIR/$suite.json")
done

status_json=$(awk -F'\t' 'NF>=2 {printf "%s{\"suite\":\"%s\",\"status\":\"%s\",\"duration_secs\":%s}", (n++ ? "," : ""), $1, $2, ($3 == "" ? 0 : $3)} END{printf ""}' "$STATUS_FILE")
status_json="[${status_json}]"

jq -n \
  "${merge_args[@]}" \
  --arg run_id "$RUN_ID" \
  --arg finished "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --argjson statuses "$status_json" \
  --arg suites "$SUITE_ORDER" '
  {
    run_id: $run_id,
    finished_at: $finished,
    suites_run: $statuses,
    results: (
      reduce ($suites | split(" "))[] as $s ({};
        ($s | gsub("-"; "_")) as $k |
        (if ($ARGS.named["s_" + $k] // null) then . + {($s): $ARGS.named["s_" + $k][0]} else . end))
    )
  }' >"$RUN_DIR/report.json"

# ---------------------------------------------------------------- summary.md --
{
  echo "# Harvester performance report"
  echo
  jq -r '"- **Run ID**: `\(.run_id)`"' "$RUN_DIR/report.json"
  jq -r '"- **Finished**: \(.finished_at)"' "$RUN_DIR/report.json"
  if [ -f "$RUN_DIR/cluster-info.json" ]; then
    jq -r '"- **Cluster**: `\(.context)` (\(.api_server))"' "$RUN_DIR/cluster-info.json"
    jq -r '"- **Harvester**: \(.versions.harvester) / Kubernetes \(.versions.kubernetes)"' "$RUN_DIR/cluster-info.json"
  fi
  echo
  echo "| Suite | Status | Duration |"
  echo "| --- | --- | --- |"
  jq -r '.suites_run[] | "| \(.suite) | \(.status) | \(.duration_secs)s |"' "$RUN_DIR/report.json"
  echo
  for suite in $SUITE_ORDER; do
    [ -f "$RUN_DIR/$suite.md" ] || continue
    cat "$RUN_DIR/$suite.md"
    echo
  done
  echo "---"
  echo
  echo "Raw JSON: \`report.json\` (merged) and \`<suite>.json\` (per suite)."
} >"$RUN_DIR/summary.md"

# ----------------------------------------------------------- terminal recap --
if [ "${QUIET:-false}" != "true" ]; then
  printf '\n%s\n' "${C_BOLD:-}================================ SUMMARY ================================${C_RESET:-}"
  jq -r '.suites_run[] | "  \(.suite): \(.status) (\(.duration_secs)s)"' "$RUN_DIR/report.json"
  echo
  if [ -f "$RUN_DIR/node-capacity.json" ]; then
    jq -L "$PERF_HOME/lib" -r 'include "quantity"; .totals |
      "  Capacity   : \(.nodes) node(s), \(.cpu_allocatable_millicores | human_cores) cores, \(.memory_allocatable_bytes | human_bytes) RAM, \(.longhorn_max_bytes | human_bytes) Longhorn"' \
      "$RUN_DIR/node-capacity.json"
  fi
  if [ -f "$RUN_DIR/controlplane-resources.json" ]; then
    jq -L "$PERF_HOME/lib" -r 'include "quantity";
      "  Ctrl plane : \(.totals.pods) pods requesting \(.totals.cpu_request_millicores | human_cores) cores (\(.totals.cpu_request_pct_of_allocatable | dash)%) and \(.totals.memory_request_bytes | human_bytes) (\(.totals.memory_request_pct_of_allocatable | dash)%)"' \
      "$RUN_DIR/controlplane-resources.json"
  fi
  if [ -f "$RUN_DIR/etcd-benchmark.json" ]; then
    jq -r '
      def ms: if . == null then "n/a" else "\(. * 1000 | .*100|round/100)ms" end;
      ([.scenarios[] | select(.name == "write-concurrent") | .requests_per_sec] | first) as $w |
      "  etcd       : \($w // 0 | floor) writes/s concurrent, WAL fsync p99 \(.disk_metrics.wal_fsync_p99_secs | ms) (\(.assessment.wal_fsync_p99))"' \
      "$RUN_DIR/etcd-benchmark.json"
  fi
  if [ -f "$RUN_DIR/vm-density.json" ]; then
    jq -r '"  VM density : \(.result.max_stable_vms) VMs (\(.config.vm_cpu) vCPU / \(.config.vm_memory)) - \(.result.stopped_because)"' \
      "$RUN_DIR/vm-density.json"
  fi
  echo
  printf '  Report: %s\n' "$RUN_DIR/summary.md"
  printf '%s\n' "${C_BOLD:-}=========================================================================${C_RESET:-}"
fi
