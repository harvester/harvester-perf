#!/usr/bin/env bash
# Suite: node-capacity - CPU, memory and storage capacity/usage of every Harvester host.
set -euo pipefail

PERF_HOME="${PERF_HOME:-/opt/harvester-perf}"
# shellcheck source=../lib/common.sh
. "$PERF_HOME/lib/common.sh"
# shellcheck source=../lib/k8s.sh
. "$PERF_HOME/lib/k8s.sh"

SUITE=node-capacity
tmp=$(mktmp)
trap 'rm -rf "$tmp"' EXIT

step "Suite: $SUITE"

kubectl get nodes -ojson >"$tmp/nodes.json"
kubectl get pods -ojson -A --field-selector=status.phase=Running >"$tmp/pods.json"

echo '{"items":[]}' >"$tmp/nodemetrics.json"
if metrics_available; then
  kubectl get --raw /apis/metrics.k8s.io/v1beta1/nodes >"$tmp/nodemetrics.json" 2>/dev/null || true
fi

echo '{"items":[]}' >"$tmp/lhnodes.json"
if has_crd nodes.longhorn.io; then
  kubectl -n longhorn-system get nodes.longhorn.io -ojson >"$tmp/lhnodes.json" 2>/dev/null || true
fi

echo '{"items":[]}' >"$tmp/blockdevices.json"
if has_crd blockdevices.harvesterhci.io; then
  kubectl get blockdevices.harvesterhci.io -ojson -A >"$tmp/blockdevices.json" 2>/dev/null || true
fi

jqp -n \
  --slurpfile ns "$tmp/nodes.json" \
  --slurpfile pods "$tmp/pods.json" \
  --slurpfile nm "$tmp/nodemetrics.json" \
  --slurpfile lh "$tmp/lhnodes.json" \
  --slurpfile bd "$tmp/blockdevices.json" '
  include "quantity";
  ($ns[0].items) as $nodes |
  ($pods[0].items) as $pods |
  ($nm[0].items // []) as $nm |
  ($lh[0].items // []) as $lh |
  ($bd[0].items // []) as $bd |

  def node_requests($name):
    [$pods[] | select(.spec.nodeName == $name) | .spec.containers[]?.resources.requests] as $r
    | { cpu_millicores: ([$r[]?.cpu | cpu_millicores] | add // 0),
        memory_bytes:   ([$r[]?.memory | mem_bytes] | add // 0) };

  def node_limits($name):
    [$pods[] | select(.spec.nodeName == $name) | .spec.containers[]?.resources.limits] as $r
    | { cpu_millicores: ([$r[]?.cpu | cpu_millicores] | add // 0),
        memory_bytes:   ([$r[]?.memory | mem_bytes] | add // 0) };

  {
    nodes: [ $nodes[] | .metadata.name as $name |
      (node_requests($name)) as $req |
      (node_limits($name)) as $lim |
      ($nm[] | select(.metadata.name == $name)) as $usage |
      ([$lh[] | select(.metadata.name == $name)] | first) as $lhnode |
      {
        name: $name,
        roles: [.metadata.labels | to_entries[] | select(.key | startswith("node-role.kubernetes.io/")) | .key | sub("node-role.kubernetes.io/"; "")],
        ready: (any(.status.conditions[]; .type == "Ready" and .status == "True")),
        pressure: [.status.conditions[] | select(.type != "Ready" and .status == "True") | .type],
        info: {
          os_image: .status.nodeInfo.osImage,
          kernel: .status.nodeInfo.kernelVersion,
          kubelet: .status.nodeInfo.kubeletVersion,
          container_runtime: .status.nodeInfo.containerRuntimeVersion,
          architecture: .status.nodeInfo.architecture
        },
        cpu: {
          capacity_millicores:    (.status.capacity.cpu | cpu_millicores),
          allocatable_millicores: (.status.allocatable.cpu | cpu_millicores),
          requested_millicores:   $req.cpu_millicores,
          limits_millicores:      $lim.cpu_millicores,
          used_millicores:        (if $usage then ($usage.usage.cpu | cpu_millicores) else null end),
          requested_pct:          pct($req.cpu_millicores; (.status.allocatable.cpu | cpu_millicores)),
          used_pct:               (if $usage then pct(($usage.usage.cpu | cpu_millicores); (.status.allocatable.cpu | cpu_millicores)) else null end)
        },
        memory: {
          capacity_bytes:    (.status.capacity.memory | mem_bytes),
          allocatable_bytes: (.status.allocatable.memory | mem_bytes),
          requested_bytes:   $req.memory_bytes,
          limits_bytes:      $lim.memory_bytes,
          used_bytes:        (if $usage then ($usage.usage.memory | mem_bytes) else null end),
          requested_pct:     pct($req.memory_bytes; (.status.allocatable.memory | mem_bytes)),
          used_pct:          (if $usage then pct(($usage.usage.memory | mem_bytes); (.status.allocatable.memory | mem_bytes)) else null end)
        },
        ephemeral_storage: {
          capacity_bytes:    (.status.capacity["ephemeral-storage"] | mem_bytes),
          allocatable_bytes: (.status.allocatable["ephemeral-storage"] | mem_bytes)
        },
        pods: {
          capacity: (.status.capacity.pods | tonumber),
          running: ([$pods[] | select(.spec.nodeName == $name)] | length)
        },
        longhorn_disks: (if $lhnode then
          [ ($lhnode.status.diskStatus // {}) | to_entries[] |
            .key as $disk | .value as $ds |
            {
              name: $disk,
              path: ($lhnode.spec.disks[$disk].path // null),
              type: ($lhnode.spec.disks[$disk].diskType // "filesystem"),
              schedulable: (any(($ds.conditions // [])[]; .type == "Schedulable" and .status == "True")),
              max_bytes: ($ds.storageMaximum // 0),
              available_bytes: ($ds.storageAvailable // 0),
              scheduled_bytes: ($ds.storageScheduled // 0),
              reserved_bytes: ($lhnode.spec.disks[$disk].storageReserved // 0)
            } ] else [] end),
        block_devices: [ $bd[] | select(.spec.nodeName == $name) |
          {
            name: .metadata.name,
            path: .spec.devPath,
            type: (.status.deviceStatus.details.deviceType // "unknown"),
            size_bytes: (.status.deviceStatus.capacity.sizeBytes // 0),
            provisioned: (.spec.fileSystem.provisioned // false),
            state: (.status.state // "unknown")
          } ]
      } ],
  } |
  . + { totals: {
      nodes: (.nodes | length),
      cpu_capacity_millicores:    ([.nodes[].cpu.capacity_millicores] | add // 0),
      cpu_allocatable_millicores: ([.nodes[].cpu.allocatable_millicores] | add // 0),
      cpu_requested_millicores:   ([.nodes[].cpu.requested_millicores] | add // 0),
      memory_capacity_bytes:      ([.nodes[].memory.capacity_bytes] | add // 0),
      memory_allocatable_bytes:   ([.nodes[].memory.allocatable_bytes] | add // 0),
      memory_requested_bytes:     ([.nodes[].memory.requested_bytes] | add // 0),
      longhorn_max_bytes:         ([.nodes[].longhorn_disks[].max_bytes] | add // 0),
      longhorn_available_bytes:   ([.nodes[].longhorn_disks[].available_bytes] | add // 0),
      longhorn_scheduled_bytes:   ([.nodes[].longhorn_disks[].scheduled_bytes] | add // 0),
      running_pods:               ([.nodes[].pods.running] | add // 0),
      pod_capacity:               ([.nodes[].pods.capacity] | add // 0)
  } }' >"$RUN_DIR/$SUITE.json"

md_init "$SUITE" "Host capacity"
{
  echo "| Node | Roles | Ready | CPU (alloc) | CPU req | CPU used | Memory (alloc) | Mem req | Mem used | Pods |"
  echo "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |"
} >>"$RUN_DIR/$SUITE.md"
jqp -r 'include "quantity";
  .nodes[] |
  "| \(.name) | \(.roles | join(",")) | \(if .ready then "yes" else "NO" end) | \(.cpu.allocatable_millicores | human_cores) | \(.cpu.requested_millicores | human_cores) (\(.cpu.requested_pct | dash)%) | \(if .cpu.used_millicores then "\(.cpu.used_millicores | human_cores) (\(.cpu.used_pct)%)" else "n/a" end) | \(.memory.allocatable_bytes | human_bytes) | \(.memory.requested_bytes | human_bytes) (\(.memory.requested_pct | dash)%) | \(if .memory.used_bytes then "\(.memory.used_bytes | human_bytes) (\(.memory.used_pct)%)" else "n/a" end) | \(.pods.running)/\(.pods.capacity) |"
' "$RUN_DIR/$SUITE.json" >>"$RUN_DIR/$SUITE.md"
md ""

if [ "$(jq '[.nodes[].longhorn_disks[]] | length' "$RUN_DIR/$SUITE.json")" -gt 0 ]; then
  md "### Longhorn disks"
  md ""
  {
    echo "| Node | Disk path | Type | Schedulable | Max | Available | Scheduled |"
    echo "| --- | --- | --- | --- | --- | --- | --- |"
  } >>"$RUN_DIR/$SUITE.md"
  jqp -r 'include "quantity";
    .nodes[] | .name as $n | .longhorn_disks[] |
    "| \($n) | \(.path | dash) | \(.type) | \(if .schedulable then "yes" else "NO" end) | \(.max_bytes | human_bytes) | \(.available_bytes | human_bytes) | \(.scheduled_bytes | human_bytes) |"
  ' "$RUN_DIR/$SUITE.json" >>"$RUN_DIR/$SUITE.md"
  md ""
fi

if [ "$(jq '[.nodes[].block_devices[]] | length' "$RUN_DIR/$SUITE.json")" -gt 0 ]; then
  md "### Block devices"
  md ""
  {
    echo "| Node | Device | Type | Size | Provisioned | State |"
    echo "| --- | --- | --- | --- | --- | --- |"
  } >>"$RUN_DIR/$SUITE.md"
  jqp -r 'include "quantity";
    .nodes[] | .name as $n | .block_devices[] |
    "| \($n) | \(.path | dash) | \(.type) | \(.size_bytes | human_bytes) | \(.provisioned) | \(.state) |"
  ' "$RUN_DIR/$SUITE.json" >>"$RUN_DIR/$SUITE.md"
  md ""
fi

jqp -r 'include "quantity"; .totals |
  "  Cluster: \(.nodes) node(s), \(.cpu_allocatable_millicores | human_cores) allocatable cores, \(.memory_allocatable_bytes | human_bytes) allocatable memory",
  "  Longhorn: \(.longhorn_available_bytes | human_bytes) available of \(.longhorn_max_bytes | human_bytes)",
  "  Pods: \(.running_pods) running of \(.pod_capacity) capacity"' "$RUN_DIR/$SUITE.json"
ok "$SUITE complete"
