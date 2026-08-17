# harvester-perf

A containerised suite of performance, capacity and resource-usage benchmarks for
[SUSE Harvester](https://harvesterhci.io) clusters. Everything runs from a single
image driven by your local `~/.kube/config` — nothing is installed on the
Harvester nodes.

```bash
make build
docker run --rm --network host \
  -v ~/.kube/config:/root/.kube/config:ro \
  -v "$PWD/results:/results" \
  -e PERF_UID=$(id -u) -e PERF_GID=$(id -g) \
  harvester-perf:dev run
```

Results land in `results/<RUN_ID>/`:

| File | Contents |
| --- | --- |
| `summary.md` | Human-readable report, all suites |
| `report.json` | Merged machine-readable result |
| `<suite>.json` | Per-suite raw data (stable schema, diffable across runs) |
| `run.log` | Full console transcript |
| `etcd-raw/` | Unparsed etcd benchmark output |

> `--network host` is only needed when your kubeconfig points at `127.0.0.1`
> (e.g. an SSH tunnel). With a routable API server address you can drop it.

## Suites

| Suite | What it measures | Mutates the cluster |
| --- | --- | --- |
| `cluster-info` | Harvester / Kubernetes / KubeVirt / Longhorn versions, node topology | no |
| `node-capacity` | Per-host CPU, memory, ephemeral storage, Longhorn disks, block devices, live usage | no |
| `etcd-benchmark` | Upstream etcd `benchmark` tool + WAL fsync / backend commit latency | short-lived helper pod |
| `controlplane-resources` | Requests, limits and live usage of every pod in the control plane namespaces, as a share of cluster allocatable | no |
| `vm-density` | How many Harvester VMs can run before the control plane degrades | **yes — creates VMs** |

```bash
harvester-perf run                          # default: everything except vm-density
harvester-perf run all                      # including vm-density
harvester-perf run node-capacity etcd-benchmark
harvester-perf list
harvester-perf report /results/20260817-101500   # re-render an existing run
```

### `etcd-benchmark`

etcd on Harvester listens on the node's host network and is protected by client
certificates that only exist on disk on the node. The suite therefore:

1. finds the node labelled `node-role.kubernetes.io/etcd=true`,
2. schedules a short-lived `hostNetwork` pod there with `/var/lib/rancher`
   mounted read-only (RKE2 and K3s layouts are both detected),
3. streams the statically linked `etcdctl` and `benchmark` binaries **from this
   image** into that pod (so no registry push is required),
4. runs the scenarios below, scrapes `etcd_disk_*` histograms from the node's
   plain-HTTP metrics port, and deletes the pod.

| Scenario | Shape |
| --- | --- |
| `write-serial` | 1 conn / 1 client sequential puts — latency floor |
| `write-concurrent` | 100 conns / 500 clients puts — throughput |
| `read-linearizable` | 1 conn / 1 client `range --consistency=l` |
| `read-serializable` | 100 conns / 500 clients `range --consistency=s` |
| `etcdctl check perf` | upstream pass/fail verdict (~60s, opt out with `ETCD_CHECK_PERF=false`) |

Disk verdicts use the upstream guidance: WAL fsync p99 ≤ 10 ms, backend commit
p99 ≤ 25 ms. Both thresholds are tunable.

The helper pod uses `registry.suse.com/bci/bci-base:15.6` by default; override
with `ETCD_BENCH_IMAGE` if your nodes cannot reach that registry.

### `vm-density`

Creates VMs the way Harvester itself does — a `VirtualMachineImage` plus a
`kubevirt.io/v1 VirtualMachine` whose root disk is a Longhorn PVC declared in the
`harvesterhci.io/volumeClaimTemplates` annotation — so both the control plane and
the storage data plane are exercised.

VMs are added in batches. After each batch the suite settles, samples cluster
health, and evaluates the gates below. The run stops at the first tripped gate
and reports the last healthy VM count.

| Gate | Default | Env var |
| --- | --- | --- |
| All VMs reach `Running` within the batch timeout | 420s | `VMD_BATCH_TIMEOUT` |
| API server pod-list latency | ≤ 2000 ms | `VMD_MAX_API_LATENCY_MS` |
| New control plane container restarts | 0 | `VMD_MAX_NEW_RESTARTS` |
| Control plane pods all ready | — | — |
| Nodes `Ready`, no `*Pressure` conditions | — | — |
| Pending pods | 0 | — |
| Cluster memory commitment | ≤ 85 % of allocatable | `VMD_MAX_MEM_COMMIT_PCT` |
| Cluster CPU usage | ≤ 90 % of allocatable | `VMD_MAX_CPU_USED_PCT` |
| Degraded / faulted Longhorn volumes | 0 | `VMD_ALLOW_DEGRADED_VOLUMES` |

The memory commitment ceiling exists so the test stops before it can push a
single-node cluster into OOM. **Run this suite against a test cluster.**

VMs and the imported image are deleted on exit (including on Ctrl-C) unless
`VMD_KEEP=true`.

## Configuration

All configuration is via environment variables passed with `docker run -e`.

### General

| Variable | Default | Purpose |
| --- | --- | --- |
| `KUBECONFIG` | `/root/.kube/config` | Kubeconfig inside the container |
| `RESULTS_DIR` | `/results` | Parent directory for run directories |
| `RUN_ID` | UTC timestamp | Name of this run |
| `FAIL_FAST` | `false` | Abort the run on the first failing suite |
| `PERF_UID` / `PERF_GID` | unset | `chown` results on exit so they are not root-owned |
| `NO_COLOR` | unset | Disable ANSI colour |
| `KUBECTL_TIMEOUT` | `60s` | Per-request timeout |

### `controlplane-resources`

| Variable | Default |
| --- | --- |
| `CP_NAMESPACES` | `kube-system harvester-system longhorn-system` |
| `CP_TOP_N` | `15` |

### `etcd-benchmark`

| Variable | Default |
| --- | --- |
| `ETCD_BENCH_NODE` | auto-detected etcd node |
| `ETCD_BENCH_NAMESPACE` | `kube-system` |
| `ETCD_BENCH_IMAGE` | `registry.suse.com/bci/bci-base:15.6` |
| `ETCD_ENDPOINTS` | `https://127.0.0.1:2379` |
| `ETCD_METRICS_PORT` | `2381` |
| `ETCD_SMALL_TOTAL` / `ETCD_LARGE_TOTAL` | `10000` / `50000` |
| `ETCD_LARGE_CONNS` / `ETCD_LARGE_CLIENTS` | `100` / `500` |
| `ETCD_VAL_SIZE` | `256` |
| `ETCD_CHECK_PERF` | `true` |
| `ETCD_WAL_FSYNC_P99_THRESHOLD` | `0.010` |
| `ETCD_BACKEND_COMMIT_P99_THRESHOLD` | `0.025` |
| `ETCD_KEEP_POD` | `false` |

### `vm-density`

| Variable | Default |
| --- | --- |
| `VM_NAMESPACE` | `harvester-perf` |
| `VM_IMAGE_URL` | cirros 0.6.2 (small and fast to boot) |
| `VM_IMAGE_NAME` | unset — reuse an existing `VirtualMachineImage` instead of importing |
| `VM_STORAGE_CLASS` | `longhorn-<image>` |
| `VMD_MAX_VMS` | `20` |
| `VMD_BATCH_SIZE` | `2` |
| `VMD_CPU` / `VMD_MEMORY` / `VMD_DISK` | `1` / `1Gi` / `10Gi` |
| `VMD_SETTLE_SECS` | `30` |
| `VMD_KEEP` | `false` |

Use a realistic guest image for meaningful numbers, e.g.:

```bash
-e VM_IMAGE_URL=https://download.opensuse.org/repositories/Cloud:/Images:/Leap_15.6/images/openSUSE-Leap-15.6.x86_64-NoCloud.qcow2 \
-e VMD_CPU=2 -e VMD_MEMORY=4Gi -e VMD_MAX_VMS=40
```

## Requirements

- A kubeconfig with cluster-admin on the target Harvester cluster (the etcd suite
  creates a privileged `hostNetwork` pod; the density suite creates VMs).
- `metrics.k8s.io` on the cluster for live usage numbers. Without it the suites
  still run and report requests/limits, with usage columns as `n/a`.

## Adding a suite

Drop `suites/<name>.sh` in place. It runs as its own process with `RUN_ID` and
`RUN_DIR` exported; source `lib/common.sh` and `lib/k8s.sh` the way the existing
suites do. It must produce:

- `$RUN_DIR/<name>.json` — the machine-readable result
- `$RUN_DIR/<name>.md` — a markdown fragment (use `md_init` / `md`)

Then add the name to `ALL_SUITES` in `entrypoint.sh`.
