#!/usr/bin/env bash
# Suite: cluster-info - identify the cluster under test and the component versions.
set -euo pipefail

PERF_HOME="${PERF_HOME:-/opt/harvester-perf}"
# shellcheck source=../lib/common.sh
. "$PERF_HOME/lib/common.sh"
# shellcheck source=../lib/k8s.sh
. "$PERF_HOME/lib/k8s.sh"

SUITE=cluster-info
tmp=$(mktmp); trap 'rm -rf "$tmp"' EXIT

step "Suite: $SUITE"

kubej nodes > "$tmp/nodes.json"

jsonpath() { kube get "$@" 2>/dev/null || true; }

harvester_version=$(jsonpath settings.harvesterhci.io server-version -o jsonpath='{.value}')
if [ -z "$harvester_version" ]; then
  harvester_version=$(jsonpath deployment harvester -n harvester-system -o jsonpath='{.spec.template.spec.containers[0].image}')
fi
kubevirt_version=$(jsonpath kubevirt -n harvester-system kubevirt -o jsonpath='{.status.observedKubeVirtVersion}')
longhorn_version=$(jsonpath settings.longhorn.io -n longhorn-system current-longhorn-version -o jsonpath='{.value}')
rancher_version=$(jsonpath settings.management.cattle.io server-version -o jsonpath='{.value}')

server_version=$(kube version -o json | jq -r '.serverVersion.gitVersion // "unknown"')
client_version=$(kube version -o json | jq -r '.clientVersion.gitVersion // "unknown"')

metrics=false; metrics_available && metrics=true
harvester=false; is_harvester && harvester=true

jqp -n \
  --slurpfile nodes "$tmp/nodes.json" \
  --arg run_id "$RUN_ID" \
  --arg started "$(now_ts)" \
  --arg context "$(current_context)" \
  --arg server "$(api_server)" \
  --arg k8s "$server_version" \
  --arg kubectl "$client_version" \
  --arg harvester_version "${harvester_version:-unknown}" \
  --arg kubevirt "${kubevirt_version:-unknown}" \
  --arg longhorn "${longhorn_version:-unknown}" \
  --arg rancher "${rancher_version:-unknown}" \
  --argjson metrics "$metrics" \
  --argjson harvester "$harvester" '
  include "quantity";
  ($nodes[0].items) as $n |
  {
    run_id: $run_id,
    started_at: $started,
    context: $context,
    api_server: $server,
    is_harvester: $harvester,
    metrics_api: $metrics,
    versions: {
      harvester: $harvester_version,
      kubernetes: $k8s,
      kubectl: $kubectl,
      kubevirt: $kubevirt,
      longhorn: $longhorn,
      rancher: $rancher,
      node_distro: ($n[0].status.nodeInfo.osImage // "unknown"),
      kernel: ($n[0].status.nodeInfo.kernelVersion // "unknown"),
      container_runtime: ($n[0].status.nodeInfo.containerRuntimeVersion // "unknown")
    },
    nodes: {
      total: ($n | length),
      ready: ([$n[] | select(any(.status.conditions[]; .type == "Ready" and .status == "True"))] | length),
      names: [$n[].metadata.name]
    },
    namespaces_present: {}
  }' > "$RUN_DIR/$SUITE.json"

md_init "$SUITE" "Cluster under test"
{
  echo "| Property | Value |"
  echo "| --- | --- |"
} >> "$RUN_DIR/$SUITE.md"
jqp -r '
  ["Context", .context],
  ["API server", .api_server],
  ["Harvester", .versions.harvester],
  ["Kubernetes", .versions.kubernetes],
  ["KubeVirt", .versions.kubevirt],
  ["Longhorn", .versions.longhorn],
  ["Rancher", .versions.rancher],
  ["Node OS", .versions.node_distro],
  ["Kernel", .versions.kernel],
  ["Container runtime", .versions.container_runtime],
  ["Nodes", "\(.nodes.ready)/\(.nodes.total) ready"],
  ["metrics.k8s.io", (if .metrics_api then "available" else "NOT available" end)]
  | "| \(.[0]) | \(.[1]) |"' "$RUN_DIR/$SUITE.json" >> "$RUN_DIR/$SUITE.md"
md ""

jqp -r '"  Harvester \(.versions.harvester) | Kubernetes \(.versions.kubernetes) | \(.nodes.ready)/\(.nodes.total) nodes ready"' "$RUN_DIR/$SUITE.json"
[ "$harvester" = true ] || warn "no harvesterhci.io CRDs found - this does not look like a Harvester cluster"
[ "$metrics" = true ]   || warn "metrics.k8s.io is unavailable; live usage numbers will be omitted"
ok "$SUITE complete"
