#!/usr/bin/env bash
# Suite: controlplane-resources - what the Harvester control plane costs in CPU/memory.
#
# Compares declared requests/limits against live usage for every pod in the
# control plane namespaces, and expresses the total as a share of cluster
# allocatable capacity (i.e. how much of the box is left for guest VMs).
set -euo pipefail

PERF_HOME="${PERF_HOME:-/opt/harvester-perf}"
# shellcheck source=../lib/common.sh
. "$PERF_HOME/lib/common.sh"
# shellcheck source=../lib/k8s.sh
. "$PERF_HOME/lib/k8s.sh"

SUITE=controlplane-resources
tmp=$(mktmp); trap 'rm -rf "$tmp"' EXIT

CP_NAMESPACES="${CP_NAMESPACES:-kube-system harvester-system longhorn-system}"
CP_TOP_N="${CP_TOP_N:-15}"

step "Suite: $SUITE"
info "namespaces: $CP_NAMESPACES"

: > "$tmp/pods.ndjson"
: > "$tmp/metrics.ndjson"
present=()
for ns in $CP_NAMESPACES; do
  if ! has_ns "$ns"; then
    warn "namespace $ns not found, skipping"
    continue
  fi
  present+=("$ns")
  kubej pods -n "$ns" | jq -c '.items[]' >> "$tmp/pods.ndjson"
  if metrics_available; then
    raw "/apis/metrics.k8s.io/v1beta1/namespaces/$ns/pods" 2>/dev/null | jq -c '.items[]?' >> "$tmp/metrics.ndjson" || true
  fi
done
[ "${#present[@]}" -gt 0 ] || die "none of the requested namespaces exist: $CP_NAMESPACES"

kubej nodes > "$tmp/nodes.json"
printf '%s\n' "${present[@]}" | jq -R . | jq -s . > "$tmp/ns.json"

jqp -n \
  --slurpfile pods "$tmp/pods.ndjson" \
  --slurpfile metrics "$tmp/metrics.ndjson" \
  --slurpfile nodes "$tmp/nodes.json" \
  --slurpfile namespaces "$tmp/ns.json" \
  --argjson top "$CP_TOP_N" '
  include "quantity";
  ($pods) as $pods |
  ($metrics) as $metrics |
  ($namespaces[0]) as $namespaces |
  ($nodes[0].items) as $nodes |

  ([$nodes[].status.allocatable.cpu | cpu_millicores] | add // 0) as $alloc_cpu |
  ([$nodes[].status.allocatable.memory | mem_bytes] | add // 0) as $alloc_mem |

  # Flatten to one record per container.
  [ $pods[] |
    .metadata.namespace as $ns | .metadata.name as $pod |
    (.status.phase) as $phase |
    ([$metrics[] | select(.metadata.namespace == $ns and .metadata.name == $pod)] | first) as $m |
    (.spec.containers[]) as $c |
    ([($m.containers // [])[] | select(.name == $c.name)] | first) as $cm |
    {
      namespace: $ns,
      pod: $pod,
      container: $c.name,
      phase: $phase,
      image: ($c.image | split("/") | last),
      cpu_request_millicores: ($c.resources.requests.cpu | cpu_millicores),
      cpu_limit_millicores:   (if $c.resources.limits.cpu then ($c.resources.limits.cpu | cpu_millicores) else null end),
      memory_request_bytes:   ($c.resources.requests.memory | mem_bytes),
      memory_limit_bytes:     (if $c.resources.limits.memory then ($c.resources.limits.memory | mem_bytes) else null end),
      cpu_used_millicores:    (if $cm then ($cm.usage.cpu | cpu_millicores) else null end),
      memory_used_bytes:      (if $cm then ($cm.usage.memory | mem_bytes) else null end),
      restarts: ([(.status.containerStatuses // [])[] | select(.name == $c.name) | .restartCount] | add // 0)
    } |
    . + {
      cpu_request_headroom_pct:  (if .cpu_used_millicores != null then pct(.cpu_used_millicores; .cpu_request_millicores) else null end),
      memory_request_headroom_pct: (if .memory_used_bytes != null then pct(.memory_used_bytes; .memory_request_bytes) else null end)
    }
  ] as $containers |

  def agg($sel):
    ($containers | map(select($sel))) as $c |
    {
      pods: ($c | map(.pod) | unique | length),
      containers: ($c | length),
      cpu_request_millicores: ($c | map(.cpu_request_millicores) | add // 0),
      cpu_limit_millicores:   ($c | map(.cpu_limit_millicores // 0) | add // 0),
      cpu_used_millicores:    ($c | map(select(.cpu_used_millicores != null) | .cpu_used_millicores) | add),
      memory_request_bytes:   ($c | map(.memory_request_bytes) | add // 0),
      memory_limit_bytes:     ($c | map(.memory_limit_bytes // 0) | add // 0),
      memory_used_bytes:      ($c | map(select(.memory_used_bytes != null) | .memory_used_bytes) | add),
      restarts:               ($c | map(.restarts) | add // 0),
      containers_without_cpu_request:    ($c | map(select(.cpu_request_millicores == 0)) | length),
      containers_without_memory_request: ($c | map(select(.memory_request_bytes == 0)) | length),
      containers_without_limits:         ($c | map(select(.cpu_limit_millicores == null and .memory_limit_bytes == null)) | length)
    };

  {
    namespaces: $namespaces,
    cluster_allocatable: {cpu_millicores: $alloc_cpu, memory_bytes: $alloc_mem},
    per_namespace: [ $namespaces[] | . as $ns | {namespace: $ns} + agg(.namespace == $ns) ],
    totals: (agg(true) + {
      cpu_request_pct_of_allocatable: pct(($containers | map(.cpu_request_millicores) | add // 0); $alloc_cpu),
      memory_request_pct_of_allocatable: pct(($containers | map(.memory_request_bytes) | add // 0); $alloc_mem),
      cpu_used_pct_of_allocatable: pct(($containers | map(.cpu_used_millicores // 0) | add // 0); $alloc_cpu),
      memory_used_pct_of_allocatable: pct(($containers | map(.memory_used_bytes // 0) | add // 0); $alloc_mem)
    }),
    top_cpu:    [ $containers | sort_by(-(.cpu_used_millicores // .cpu_request_millicores))[:$top][] ],
    top_memory: [ $containers | sort_by(-(.memory_used_bytes // .memory_request_bytes))[:$top][] ],
    no_requests: [ $containers[] | select(.cpu_request_millicores == 0 or .memory_request_bytes == 0) |
                   {namespace, pod, container, cpu_request_millicores, memory_request_bytes} ],
    unhealthy: [ $containers[] | select(.phase != "Running" and .phase != "Succeeded") | {namespace, pod, container, phase} ],
    restarting: [ $containers[] | select(.restarts > 0) | {namespace, pod, container, restarts} ] | sort_by(-.restarts),
    containers: $containers
  }' > "$RUN_DIR/$SUITE.json"

md_init "$SUITE" "Control plane resource footprint"
md "Namespaces measured: $(jq -r '.namespaces | map("`\(.)`") | join(", ")' "$RUN_DIR/$SUITE.json")"
md ""
{
  echo "| Namespace | Pods | Ctrs | CPU req | CPU limit | CPU used | Mem req | Mem limit | Mem used | Restarts |"
  echo "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |"
} >> "$RUN_DIR/$SUITE.md"
jqp -r 'include "quantity";
  (.per_namespace[], (.totals + {namespace: "**total**"})) |
  "| \(.namespace) | \(.pods) | \(.containers) | \(.cpu_request_millicores | human_cores) | \(.cpu_limit_millicores | human_cores) | \(if .cpu_used_millicores then (.cpu_used_millicores | human_cores) else "n/a" end) | \(.memory_request_bytes | human_bytes) | \(.memory_limit_bytes | human_bytes) | \(if .memory_used_bytes then (.memory_used_bytes | human_bytes) else "n/a" end) | \(.restarts) |"
' "$RUN_DIR/$SUITE.json" >> "$RUN_DIR/$SUITE.md"
md ""
md "### Share of cluster allocatable consumed by the control plane"
md ""
{
  echo "| Resource | Requested | Used | Cluster allocatable | Requested % | Used % |"
  echo "| --- | --- | --- | --- | --- | --- |"
} >> "$RUN_DIR/$SUITE.md"
jqp -r 'include "quantity";
  "| CPU | \(.totals.cpu_request_millicores | human_cores) | \(.totals.cpu_used_millicores // 0 | human_cores) | \(.cluster_allocatable.cpu_millicores | human_cores) | \(.totals.cpu_request_pct_of_allocatable | dash)% | \(.totals.cpu_used_pct_of_allocatable | dash)% |",
  "| Memory | \(.totals.memory_request_bytes | human_bytes) | \(.totals.memory_used_bytes // 0 | human_bytes) | \(.cluster_allocatable.memory_bytes | human_bytes) | \(.totals.memory_request_pct_of_allocatable | dash)% | \(.totals.memory_used_pct_of_allocatable | dash)% |"
' "$RUN_DIR/$SUITE.json" >> "$RUN_DIR/$SUITE.md"
md ""
md "### Top consumers"
md ""
{
  echo "| Namespace | Pod | Container | CPU used | CPU req | Mem used | Mem req |"
  echo "| --- | --- | --- | --- | --- | --- | --- |"
} >> "$RUN_DIR/$SUITE.md"
jqp -r 'include "quantity";
  .top_memory[] |
  "| \(.namespace) | \(.pod) | \(.container) | \(if .cpu_used_millicores then (.cpu_used_millicores | human_cores) else "n/a" end) | \(.cpu_request_millicores | human_cores) | \(if .memory_used_bytes then (.memory_used_bytes | human_bytes) else "n/a" end) | \(.memory_request_bytes | human_bytes) |"
' "$RUN_DIR/$SUITE.json" >> "$RUN_DIR/$SUITE.md"
md ""
nr=$(jq '.no_requests | length' "$RUN_DIR/$SUITE.json")
if [ "$nr" -gt 0 ]; then
  md "### Containers without CPU and/or memory requests ($nr)"
  md ""
  md "Unrequested containers are invisible to the scheduler and make VM density planning unreliable."
  md ""
  jq -r '.no_requests[] | "- `\(.namespace)/\(.pod)` container `\(.container)`"' "$RUN_DIR/$SUITE.json" >> "$RUN_DIR/$SUITE.md"
  md ""
fi

jqp -r 'include "quantity";
  "  Control plane: \(.totals.pods) pods / \(.totals.containers) containers across \(.namespaces | length) namespaces",
  "  Requests: \(.totals.cpu_request_millicores | human_cores) cores (\(.totals.cpu_request_pct_of_allocatable | dash)% of allocatable), \(.totals.memory_request_bytes | human_bytes) memory (\(.totals.memory_request_pct_of_allocatable | dash)%)",
  "  Live usage: \(.totals.cpu_used_millicores // 0 | human_cores) cores (\(.totals.cpu_used_pct_of_allocatable | dash)%), \(.totals.memory_used_bytes // 0 | human_bytes) memory (\(.totals.memory_used_pct_of_allocatable | dash)%)"
' "$RUN_DIR/$SUITE.json"
[ "$nr" -eq 0 ] || warn "$nr container(s) declare no CPU and/or memory request"
ok "$SUITE complete"
