# shellcheck shell=bash
# kubectl helpers shared by the harvester-perf suites.

KUBECTL_TIMEOUT="${KUBECTL_TIMEOUT:-60s}"

kube()  { kubectl --request-timeout="$KUBECTL_TIMEOUT" "$@"; }
kubej() { kube get "$@" -o json; }
raw()   { kube get --raw "$1"; }

current_context() { kube config current-context 2>/dev/null || echo "unknown"; }
api_server()      { kube config view --minify -o jsonpath='{.clusters[0].cluster.server}' 2>/dev/null || echo "unknown"; }

require_cluster() {
  kube version -o json >/dev/null 2>&1 \
    || die "cannot reach the Kubernetes API at $(api_server) (context: $(current_context)). Check the mounted kubeconfig."
}

has_crd()  { kube get crd "$1" >/dev/null 2>&1; }
has_ns()   { kube get namespace "$1" >/dev/null 2>&1; }
has_api()  { kube api-resources --api-group="$1" -o name >/dev/null 2>&1; }

metrics_available() { raw /apis/metrics.k8s.io/v1beta1 >/dev/null 2>&1; }

is_harvester() { has_crd virtualmachineimages.harvesterhci.io; }

# Milliseconds taken by an API call, or -1 on failure.
api_latency_ms() {
  local path="${1:-/api/v1/namespaces/default/pods?limit=500}" start end
  start=$(epoch_ms)
  if raw "$path" >/dev/null 2>&1; then
    end=$(epoch_ms); echo $(( end - start ))
  else
    echo "-1"
  fi
}

# wait_for <timeout-secs> <interval-secs> <command...> - poll until the command succeeds.
wait_for() {
  local timeout="$1" interval="$2"; shift 2
  local deadline=$(( $(date +%s) + timeout ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    if "$@" >/dev/null 2>&1; then return 0; fi
    sleep "$interval"
  done
  return 1
}

# The node most likely to run etcd.
etcd_node() {
  local n
  for sel in "node-role.kubernetes.io/etcd=true" "node-role.kubernetes.io/control-plane=true" "node-role.kubernetes.io/master=true"; do
    n=$(kube get nodes -l "$sel" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
    [ -n "$n" ] && { echo "$n"; return 0; }
  done
  kube get nodes -o jsonpath='{.items[0].metadata.name}'
}
