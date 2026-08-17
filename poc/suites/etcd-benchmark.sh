#!/usr/bin/env bash
# Suite: etcd-benchmark - run the upstream etcd benchmark tool against the cluster's etcd.
#
# etcd on Harvester (RKE2) listens on the host network and is protected by client
# certificates that only exist on the node. We therefore schedule a short lived
# hostNetwork pod on the etcd node, copy the statically linked etcdctl/benchmark
# binaries from this image into it, and drive the benchmark over kubectl exec.
set -euo pipefail

PERF_HOME="${PERF_HOME:-/opt/harvester-perf}"
# shellcheck source=../lib/common.sh
. "$PERF_HOME/lib/common.sh"
# shellcheck source=../lib/k8s.sh
. "$PERF_HOME/lib/k8s.sh"

SUITE=etcd-benchmark
tmp=$(mktmp)

ETCD_BENCH_NAMESPACE="${ETCD_BENCH_NAMESPACE:-kube-system}"
ETCD_BENCH_IMAGE="${ETCD_BENCH_IMAGE:-registry.suse.com/bci/bci-base:15.6}"
ETCD_ENDPOINTS="${ETCD_ENDPOINTS:-https://127.0.0.1:2379}"
ETCD_METRICS_PORT="${ETCD_METRICS_PORT:-2381}"
ETCD_BENCH_NODE="${ETCD_BENCH_NODE:-}"
ETCD_POD_TIMEOUT="${ETCD_POD_TIMEOUT:-180}"
# Scenario sizing (kept modest so a default run does not hammer a live cluster).
ETCD_SMALL_TOTAL="${ETCD_SMALL_TOTAL:-10000}"
ETCD_LARGE_TOTAL="${ETCD_LARGE_TOTAL:-50000}"
ETCD_LARGE_CONNS="${ETCD_LARGE_CONNS:-100}"
ETCD_LARGE_CLIENTS="${ETCD_LARGE_CLIENTS:-500}"
ETCD_VAL_SIZE="${ETCD_VAL_SIZE:-256}"
ETCD_CHECK_PERF="${ETCD_CHECK_PERF:-true}"
ETCD_KEEP_POD="${ETCD_KEEP_POD:-false}"

POD="harvester-perf-etcd-$(tr -dc 'a-z0-9' </dev/urandom | head -c 6)"

cleanup() {
  if [ "$ETCD_KEEP_POD" != "true" ]; then
    kube delete pod -n "$ETCD_BENCH_NAMESPACE" "$POD" --ignore-not-found --wait=false >/dev/null 2>&1 || true
  fi
  rm -rf "$tmp"
}
trap cleanup EXIT

step "Suite: $SUITE"

node="${ETCD_BENCH_NODE:-$(etcd_node)}"
[ -n "$node" ] || die "unable to determine an etcd node"
info "etcd node: $node"

cat > "$tmp/pod.yaml" <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: $POD
  namespace: $ETCD_BENCH_NAMESPACE
  labels:
    app.kubernetes.io/name: harvester-perf
    harvester-perf/suite: etcd-benchmark
    harvester-perf/run: "$RUN_ID"
spec:
  nodeName: $node
  hostNetwork: true
  restartPolicy: Never
  terminationGracePeriodSeconds: 5
  tolerations:
    - operator: Exists
  containers:
    - name: bench
      image: $ETCD_BENCH_IMAGE
      imagePullPolicy: IfNotPresent
      command: ["/bin/bash", "-c", "sleep 3600"]
      securityContext:
        runAsUser: 0
        privileged: true
      volumeMounts:
        - name: rancher
          mountPath: /host/rancher
          readOnly: true
  volumes:
    - name: rancher
      hostPath:
        path: /var/lib/rancher
        type: Directory
EOF

info "creating benchmark pod $ETCD_BENCH_NAMESPACE/$POD"
kube apply -f "$tmp/pod.yaml" >/dev/null

pod_running() { [ "$(kube get pod -n "$ETCD_BENCH_NAMESPACE" "$POD" -o jsonpath='{.status.phase}' 2>/dev/null)" = "Running" ]; }
if ! wait_for "$ETCD_POD_TIMEOUT" 3 pod_running; then
  kube describe pod -n "$ETCD_BENCH_NAMESPACE" "$POD" 2>&1 | tail -20 || true
  die "benchmark pod did not start within ${ETCD_POD_TIMEOUT}s (image $ETCD_BENCH_IMAGE must be pullable by the node)"
fi
ok "benchmark pod running on $node"

pexec()  { kube exec -n "$ETCD_BENCH_NAMESPACE" "$POD" -- bash -c "$1"; }
pexecq() { pexec "$1" 2>/dev/null; }

# Ship the statically linked binaries from this image into the pod.
copy_binary() {
  local src="$1" dst="$2"
  if kube cp "$src" "$ETCD_BENCH_NAMESPACE/$POD:$dst" >/dev/null 2>&1 && pexecq "test -s $dst"; then
    :
  else
    info "kubectl cp unavailable, streaming $(basename "$src") via base64"
    base64 -w0 "$src" | kube exec -i -n "$ETCD_BENCH_NAMESPACE" "$POD" -- bash -c "base64 -d > $dst"
  fi
  pexec "chmod +x $dst" >/dev/null
}
info "copying etcdctl and benchmark into the pod"
copy_binary /usr/local/bin/etcdctl /tmp/etcdctl
copy_binary /usr/local/bin/benchmark /tmp/benchmark

# Locate the RKE2/K3s etcd client certificates on the host.
certdir=$(pexecq 'for d in /host/rancher/rke2/server/tls/etcd /host/rancher/k3s/server/tls/etcd; do [ -f "$d/server-ca.crt" ] && echo "$d" && break; done' | tr -d '\r')
[ -n "$certdir" ] || die "no RKE2/K3s etcd certificates under /var/lib/rancher on $node (set ETCD_ENDPOINTS/ETCD_CERT_DIR for a custom layout)"
info "etcd certificates: ${certdir#/host}"

TLS="--cacert $certdir/server-ca.crt --cert $certdir/server-client.crt --key $certdir/server-client.key"
ETCDCTL="/tmp/etcdctl --endpoints=$ETCD_ENDPOINTS $TLS"
BENCH="/tmp/benchmark --endpoints=$ETCD_ENDPOINTS --cacert=$certdir/server-ca.crt --cert=$certdir/server-client.crt --key=$certdir/server-client.key"

pexecq "$ETCDCTL endpoint health" >/dev/null || die "etcd endpoint $ETCD_ENDPOINTS is not healthy from $node"

pexecq "$ETCDCTL -w json endpoint status"  > "$tmp/status.json"  || echo '[]' > "$tmp/status.json"
pexecq "$ETCDCTL -w json endpoint health"  > "$tmp/health.json"  || echo '[]' > "$tmp/health.json"
pexecq "$ETCDCTL -w json member list"      > "$tmp/members.json" || echo '{}' > "$tmp/members.json"
jq -e . "$tmp/status.json" >/dev/null 2>&1 || echo '[]' > "$tmp/status.json"
jq -e . "$tmp/health.json" >/dev/null 2>&1 || echo '[]' > "$tmp/health.json"
jq -e . "$tmp/members.json" >/dev/null 2>&1 || echo '{}' > "$tmp/members.json"

# ---------------------------------------------------------------- scenarios --
# Run one `benchmark` invocation and append its parsed summary to scenarios.ndjson.
run_scenario() {
  local name="$1" desc="$2" cmd="$3"
  info "scenario: $name"
  local out rc=0
  out=$(pexec "$cmd" 2>&1) || rc=$?
  printf '%s\n' "$out" > "$tmp/$name.txt"
  if [ "$rc" -ne 0 ]; then
    warn "scenario $name failed (exit $rc)"
    jq -n --arg n "$name" --arg d "$desc" --arg c "$cmd" --arg o "$(printf '%s' "$out" | tail -5)" \
      '{name:$n, description:$d, command:$c, ok:false, error:$o}' >> "$tmp/scenarios.ndjson"
    return 0
  fi
  printf '%s\n' "$out" | awk -v name="$name" -v desc="$desc" -v cmd="$cmd" '
    /^[[:space:]]*Total:/         { total=$2 }
    /^[[:space:]]*Slowest:/       { slowest=$2 }
    /^[[:space:]]*Fastest:/       { fastest=$2 }
    /^[[:space:]]*Average:/       { average=$2 }
    /^[[:space:]]*Stddev:/        { stddev=$2 }
    /^[[:space:]]*Requests\/sec:/ { rps=$2 }
    /^[[:space:]]*[0-9.]+% in/    { k=$1; sub("%","",k); p[k]=$3 }
    /^Error distribution:/        { errs=1 }
    function num(v) { return (v == "" ? "null" : v+0) }
    END {
      printf "{\"name\":\"%s\",\"description\":\"%s\",\"command\":\"%s\",\"ok\":true,", name, desc, cmd
      printf "\"total_secs\":%s,\"requests_per_sec\":%s,", num(total), num(rps)
      printf "\"latency_secs\":{\"average\":%s,\"slowest\":%s,\"fastest\":%s,\"stddev\":%s,",
             num(average), num(slowest), num(fastest), num(stddev)
      printf "\"p50\":%s,\"p90\":%s,\"p95\":%s,\"p99\":%s,\"p999\":%s},",
             num(p["50"]), num(p["90"]), num(p["95"]), num(p["99"]), num(p["99.9"])
      printf "\"errors\":%s}\n", (errs ? "true" : "false")
    }' >> "$tmp/scenarios.ndjson"
  jq -r '"    \(.requests_per_sec) req/s, avg \(.latency_secs.average * 1000 | .*100|round/100)ms, p99 \(.latency_secs.p99 * 1000 | .*100|round/100)ms"' \
    <<<"$(tail -1 "$tmp/scenarios.ndjson")"
}

: > "$tmp/scenarios.ndjson"

run_scenario "write-serial" \
  "Sequential writes, 1 connection / 1 client (latency floor)" \
  "$BENCH --conns=1 --clients=1 put --key-size=8 --sequential-keys --total=$ETCD_SMALL_TOTAL --val-size=$ETCD_VAL_SIZE"

run_scenario "write-concurrent" \
  "Concurrent writes, $ETCD_LARGE_CONNS conns / $ETCD_LARGE_CLIENTS clients (throughput)" \
  "$BENCH --conns=$ETCD_LARGE_CONNS --clients=$ETCD_LARGE_CLIENTS put --key-size=8 --sequential-keys --total=$ETCD_LARGE_TOTAL --val-size=$ETCD_VAL_SIZE"

run_scenario "read-linearizable" \
  "Linearizable single-key reads, 1 conn / 1 client" \
  "$BENCH --conns=1 --clients=1 range harvester-perf-probe --consistency=l --total=$ETCD_SMALL_TOTAL"

run_scenario "read-serializable" \
  "Serializable single-key reads, $ETCD_LARGE_CONNS conns / $ETCD_LARGE_CLIENTS clients" \
  "$BENCH --conns=$ETCD_LARGE_CONNS --clients=$ETCD_LARGE_CLIENTS range harvester-perf-probe --consistency=s --total=$ETCD_LARGE_TOTAL"

check_perf='{}'
if [ "$ETCD_CHECK_PERF" = "true" ]; then
  info "scenario: etcdctl check perf (~60s)"
  cp_out=$(pexec "$ETCDCTL check perf --load=${ETCD_CHECK_PERF_LOAD:-s}" 2>&1) || true
  printf '%s\n' "$cp_out" > "$tmp/check-perf.txt"
  check_perf=$(printf '%s\n' "$cp_out" | awk '
    /PASS|FAIL/ { verdict = (index($0,"PASS") ? "PASS" : "FAIL") }
    /Throughput/ { thr=$NF }
    /Slowest request took/ { slow=$4 }
    /Stddev/ { sd=$NF }
    END { printf "{\"verdict\":\"%s\",\"raw_throughput\":\"%s\",\"slowest_request_secs\":%s}",
                 (verdict==""?"unknown":verdict), thr, (slow==""?"null":slow+0) }')
  jq -e . <<<"$check_perf" >/dev/null 2>&1 || check_perf='{"verdict":"unknown"}'
  printf '    verdict: %s\n' "$(jq -r .verdict <<<"$check_perf")"
fi

# ------------------------------------------------------------ etcd metrics --
# RKE2 exposes the plain-HTTP etcd metrics endpoint on 127.0.0.1:2381.
info "collecting etcd disk latency metrics"
pexecq "exec 3<>/dev/tcp/127.0.0.1/$ETCD_METRICS_PORT; printf 'GET /metrics HTTP/1.0\r\nHost: localhost\r\n\r\n' >&3; cat <&3" \
  > "$tmp/metrics.txt" || true

# Estimate a quantile from a Prometheus histogram's buckets. Buckets are emitted
# in ascending `le` order, so a single ordered pass is enough.
hist_quantile() {
  local metric="$1" q="$2"
  awk -v m="${metric}_bucket" -v q="$q" '
    index($0, m"{") == 1 {
      le = $0; sub(/.*le="/, "", le); sub(/".*/, "", le)
      n++; bound[n] = le; count[n] = $NF + 0
      if (count[n] > total) total = count[n]
    }
    END {
      if (n == 0 || total == 0) { print "null"; exit }
      target = q * total
      for (i = 1; i <= n; i++)
        if (count[i] >= target) {
          if (bound[i] == "+Inf") break
          printf "%.6f", bound[i] + 0; exit
        }
      print "null"
    }' "$tmp/metrics.txt" 2>/dev/null || echo null
}
hist_avg() {
  local metric="$1"
  awk -v m="$metric" '
    index($0, m"_sum") == 1 { s = $NF + 0 }
    index($0, m"_count") == 1 { c = $NF + 0 }
    END { if (c > 0) printf "%.6f", s / c; else print "null" }' "$tmp/metrics.txt" 2>/dev/null || echo null
}

metrics_json=$(jq -n \
  --argjson wal_p99 "$(hist_quantile etcd_disk_wal_fsync_duration_seconds 0.99)" \
  --argjson wal_avg "$(hist_avg etcd_disk_wal_fsync_duration_seconds)" \
  --argjson commit_p99 "$(hist_quantile etcd_disk_backend_commit_duration_seconds 0.99)" \
  --argjson commit_avg "$(hist_avg etcd_disk_backend_commit_duration_seconds)" \
  --argjson rt_p99 "$(hist_quantile etcd_network_peer_round_trip_time_seconds 0.99)" \
  '{wal_fsync_p99_secs:$wal_p99, wal_fsync_avg_secs:$wal_avg,
    backend_commit_p99_secs:$commit_p99, backend_commit_avg_secs:$commit_avg,
    peer_round_trip_p99_secs:$rt_p99}' 2>/dev/null || echo '{}')

# etcd health verdicts based on the upstream hardware guidance.
assessment=$(jq -n --argjson m "$metrics_json" \
  --argjson wal_threshold "${ETCD_WAL_FSYNC_P99_THRESHOLD:-0.010}" \
  --argjson commit_threshold "${ETCD_BACKEND_COMMIT_P99_THRESHOLD:-0.025}" '
  {
    wal_fsync_p99: (if $m.wal_fsync_p99_secs == null then "unknown"
                    elif $m.wal_fsync_p99_secs <= $wal_threshold then "ok" else "slow disk" end),
    backend_commit_p99: (if $m.backend_commit_p99_secs == null then "unknown"
                    elif $m.backend_commit_p99_secs <= $commit_threshold then "ok" else "slow disk" end),
    thresholds: {wal_fsync_p99_secs: $wal_threshold, backend_commit_p99_secs: $commit_threshold}
  }')

jq -n \
  --arg node "$node" \
  --arg endpoints "$ETCD_ENDPOINTS" \
  --slurpfile status "$tmp/status.json" \
  --slurpfile health "$tmp/health.json" \
  --slurpfile members "$tmp/members.json" \
  --slurpfile scenarios "$tmp/scenarios.ndjson" \
  --argjson check_perf "$check_perf" \
  --argjson metrics "$metrics_json" \
  --argjson assessment "$assessment" '
  {
    node: $node,
    endpoints: $endpoints,
    members: [ ($members[0].members // [])[] | {id: (.ID // .id), name: (.name // ""), client_urls: (.clientURLs // [])} ],
    endpoint_status: [ ($status[0] // [])[] | {
        endpoint: .Endpoint,
        version: .Status.version,
        db_size_bytes: .Status.dbSize,
        db_size_in_use_bytes: (.Status.dbSizeInUse // null),
        leader: (.Status.leader == .Status.header.member_id),
        raft_term: .Status.raftTerm,
        raft_index: (.Status.raftIndex // null),
        alarms: (.Status.errors // [])
      } ],
    endpoint_health: [ ($health[0] // [])[] | {endpoint: .endpoint, healthy: .health, took: .took} ],
    scenarios: $scenarios,
    check_perf: $check_perf,
    disk_metrics: $metrics,
    assessment: $assessment
  }' > "$RUN_DIR/$SUITE.json"

md_init "$SUITE" "etcd performance"
md "Benchmarked \`$ETCD_ENDPOINTS\` from node \`$node\` using the upstream etcd \`benchmark\` tool."
md ""
{
  echo "| Scenario | req/s | avg | p50 | p99 | slowest |"
  echo "| --- | --- | --- | --- | --- | --- |"
} >> "$RUN_DIR/$SUITE.md"
jq -r '.scenarios[] |
  if .ok then
    "| \(.description) | \(.requests_per_sec // 0 | floor) | \((.latency_secs.average // 0) * 1000 | .*100|round/100)ms | \((.latency_secs.p50 // 0) * 1000 | .*100|round/100)ms | \((.latency_secs.p99 // 0) * 1000 | .*100|round/100)ms | \((.latency_secs.slowest // 0) * 1000 | .*100|round/100)ms |"
  else
    "| \(.description) | FAILED | - | - | - | - |"
  end' "$RUN_DIR/$SUITE.json" >> "$RUN_DIR/$SUITE.md"
md ""
md "### Disk latency (etcd server metrics)"
md ""
{
  echo "| Metric | Value | Threshold | Verdict |"
  echo "| --- | --- | --- | --- |"
} >> "$RUN_DIR/$SUITE.md"
jq -r '
  def ms: if . == null then "n/a" else "\(. * 1000 | .*100|round/100)ms" end;
  "| WAL fsync p99 | \(.disk_metrics.wal_fsync_p99_secs | ms) | \(.assessment.thresholds.wal_fsync_p99_secs | ms) | \(.assessment.wal_fsync_p99) |",
  "| WAL fsync avg | \(.disk_metrics.wal_fsync_avg_secs | ms) | - | - |",
  "| Backend commit p99 | \(.disk_metrics.backend_commit_p99_secs | ms) | \(.assessment.thresholds.backend_commit_p99_secs | ms) | \(.assessment.backend_commit_p99) |",
  "| Backend commit avg | \(.disk_metrics.backend_commit_avg_secs | ms) | - | - |"' "$RUN_DIR/$SUITE.json" >> "$RUN_DIR/$SUITE.md"
md ""
jq -r '.endpoint_status[]? | "- db size `\(.endpoint)`: \(.db_size_bytes) bytes (in use \(.db_size_in_use_bytes // "n/a")), version \(.version)"' \
  "$RUN_DIR/$SUITE.json" >> "$RUN_DIR/$SUITE.md"
md ""
if [ "$ETCD_CHECK_PERF" = "true" ]; then
  md "\`etcdctl check perf\`: **$(jq -r '.check_perf.verdict // "unknown"' "$RUN_DIR/$SUITE.json")**"
  md ""
fi

mkdir -p "$RUN_DIR/etcd-raw"
cp "$tmp"/*.txt "$RUN_DIR/etcd-raw/" 2>/dev/null || true
ok "$SUITE complete"
