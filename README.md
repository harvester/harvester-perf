# harvester-perf

This repository contains a collection of tools, pipelines and documentation for
assessing Harvester performance and benchmark results.

`hvperf` is a CLI for running performance, capacity and benchmark test suites
against [SUSE Harvester](https://harvesterhci.io) clusters.

## Requirements

* A kubeconfig with cluster-admin on the target Harvester cluster.
* Docker (or a compatible runtime) for running build and test targets.
* Go 1.26+ only if you build outside the container.
* The `rancher-monitoring` addon, only for the metrics cases; suites skip those
  cases when it is not enabled.
* For `etcd-benchmark`'s metrics cases, the RKE2 server config
  [`etcd-expose-metrics`](https://docs.rke2.io/reference/server_config#database)
  must be set to `true`; etcd does not expose metrics otherwise.

## Concept

Suites are driven from your workstation using an ordinary kubeconfig. Nothing is
installed on the Harvester nodes; suites that need on-node access (such as the
etcd benchmark) schedule a short-lived helper job in the test namespace, which
is deleted once the run is over unless `--keep-alive` is set.

These helper pods run **privileged**, since the on-node access they need can't
be had otherwise: e.g., the etcd benchmark's job pod mounts `/var/lib/rancher` to
reach the RKE2 etcd TLS certs, and `node-capacity`'s daemonset pod uses `hostPID`
to run `nsenter`/`lsblk` against the node's disks.

Every command that talks to a cluster accepts the standard kubectl connection
flags (`--kubeconfig`, `--context`, `--namespace`, `--server`, `--as`, …), so
targeting a specific cluster works the same way it does with `kubectl`:

```bash
hvperf run node-capacity --kubeconfig ~/.kube/harvester.yaml --context prod
```

Suites that create resources put them in the `harvester-perf-system` namespace,
creating the namespace if it does not exist. Pass `--namespace` to use a
different one.

## Quick start

```bash
# build ./dist/hvperf (runs the compiler inside a container, see Building)
make go/build
sudo ln -s `pwd`/dist/hvperf /usr/local/bin/hvperf  # optional, for convenience

# install the benchmark tools (etcdctl, benchmark) into ~/.hvperf/tools
make tools   # dev only: builds ./dist/hvperf-tools-<version>.tar.gz

hvperf tool install --tool-source ./dist/hvperf-tools-<version>.tar.gz

# what can be run
hvperf list

# client and cluster versions
hvperf version

# run a single suite
hvperf run node-capacity

# run several suites (one comma-separated argument, not a space-separated list)
hvperf run node-capacity,etcd-benchmark

# run exclusive suites one at a time because they stress the cluster and may
# temporarily degrade its performance
hvperf run storage-kbench
hvperf run etcd-benchmark
```

## Commands

| Command | Description |
| --- | --- |
| `hvperf list` | List registered suites. `-o` accepts `table` (default), `name`, `json`, `yaml`. |
| `hvperf run <suite>[,<suite>...]` | Run the named suites, given as a single comma-separated argument. An unknown name fails the run. An exclusive suite cannot be run together with other suites. `-o` accepts `text` (default), `json`, `yaml`. |
| `hvperf tool install -t <tar.gz>` / `list` / `uninstall [-y]` | Install, list or remove the benchmark tools in `~/.hvperf/tools`. See [Setting up the tools](#setting-up-the-tools). |
| `hvperf version [--client-only]` | Print the client version and, unless `--client-only` is set, the cluster's server version. |

`run` also takes `--keep-alive` (default `false`), which decides what happens to
the test namespace once the run finishes. By default the namespace and
everything in it is deleted afterwards — but only when it is the default
`harvester-perf-system`. Pass `--keep-alive=true` to keep the namespace and its
resources around for inspection. A custom `--namespace` is never deleted and its
resources are not cleaned up; a warning is logged when cleanup is skipped.

Results are written to stdout; suite progress is logged to stderr, so
`hvperf run ... -o json > results.json` keeps the two streams separate. Pass
`-q`/`--quiet` to silence the progress logging, or `-v <level>` to raise it.
Results are printed even when a suite fails, so partial output is not lost.

### Setting up the tools

Some suites (e.g. `etcd-benchmark`) need extra binaries such as `etcdctl`,
and `benchmark`. `hvperf run` reads them from `~/.hvperf/tools`. Set
that directory up with `hvperf tool`:

```bash
# install from a local .tar.gz or an http(s) URL, e.g. the tarball attached to
# a release on the hvperf releases page
hvperf tool install -t https://github.com/<org>/harvester-perf/releases/download/<tag>/hvperf-tools-<version>.tar.gz
hvperf tool install --tool-source ./dist/hvperf-tools-<version>.tar.gz

# show what is installed
hvperf tool list

# remove ~/.hvperf/tools (prompts for confirmation; -y/--skip-confirm to skip)
hvperf tool uninstall
```

The tools tarball is published as a release asset on the `hvperf` releases
page. For development, `make tools` builds it locally into `./dist/` (inside a
container; honours `ETCD_VERSION`).

The archive must be a `.tar.gz` of regular files (the tool binaries); other
entry types are skipped. Files are extracted flat into the tools directory,
overwriting existing ones.

`make tools` uses the Dockerfile to build `hvperf` and the tools that the suites
need. The etcd tools are built from the etcd source at the version pinned in the
Dockerfile (`ETCD_VERSION`, currently `v3.6.14`). The runtime base is
`registry.suse.com/bci/bci-base` at `BCI_TAG` (`16.0`).

### Configuration File For Parameters Overrides

`hvperf run` reads `./hvperf.yaml` by default. Copy and edit the file at the
repo root to tune suite parameters before running:

```yaml
density:
  batchWaitTimeout: 5m     # per-batch wait timeout
  vmi:
    containerDisk: quay.io/kubevirt/cirros-container-disk-demo:latest
    memory: 90Mi
    cpu: 100m
```

Pass `--config` to point at a different file:

```bash
hvperf run density --config ~/my-cluster.yaml
```

## Development

### Test suites

Suites are either **exclusive** or non-exclusive. Exclusive suites impose stress
on the cluster through stress loaders such as `fio`, which may temporarily
degrade the cluster's overall performance and stability. They are therefore not
suitable to run alongside other suites, to avoid worsening the cluster's
condition; `hvperf run` rejects an exclusive suite combined with any other
suite. The flag is reported by `hvperf list`.

| Suite | Exclusive | What it does |
| --- | --- | --- |
| `node-capacity` | no | Assess node resource capacity: per-node OS info and disk info via a privileged `hostPID` daemonset. |
| `etcd-benchmark` | yes | Exercises the cluster's etcd from a privileged `hostNetwork` job pod. |
| `resource-footprint` | no | Measures the cluster's resource footprint: per-namespace CPU/memory usage and requests, host memory, and node allocatable, via Prometheus queries. |
| `density` | yes | Import a VM image, ramp up VMs concurrently until failure, and record VMI boot-latency percentiles (p50/p95/p99). |
| `storage-kbench` | yes | Run Longhorn's `kbench` job against a fresh PVC to benchmark bandwidth, IOPS and latency, then read the matching Longhorn metrics from Prometheus. |

### Result types

A `SuiteResult` holds one `CaseResult` per check the suite ran. Each
`CaseResult` records what it did in whichever of these it produced —
a case can populate more than one:

| Type | Use it for |
| --- | --- |
| `CmdResult` | The result of executing a command in a test case — e.g. `exec`-ing into a helper pod. `Stdout`/`Stderr` hold the command's output streams. |
| `MetricResult` | The result of a Prometheus query run in a test case — e.g. reading etcd metrics after a benchmark. `Samples` holds the returned vector, `Warnings` any promclient warnings. |
| `PodLogResult` | The logs of a pod in a test case — e.g. the `kbench` job pod's output. `Subject` names the pod, `Logs` holds its output. |
| `K8sResourceResult` | The result of querying Kubernetes/node resources in a test case — e.g. `node-capacity`'s per-node OS and disk info. `Data` is a flat map of the resource's spec/status attributes. |

### Errors

Every result type carries its own `Err`, at a different scope:

| Field | Scope |
| --- | --- |
| `SuiteResult.Err` | The suite itself failed outside any single case — e.g. setup or cleanup shared by the whole suite. |
| `CaseResult.Err` | The case failed outside a specific command/query — e.g. a namespace or daemonset never became ready. Setting it marks the case errored immediately, before its `CmdResults`/`MetricResults`/`K8sResourceResults` are even considered. |
| `CmdResult.Err` | One `exec`'d command failed. |
| `MetricResult.Err` | One Prometheus query failed. |
| `PodLogResult.Err` | Fetching one pod's logs failed. |
| `K8sResourceResult.Err` | One resource query failed (this is the one `error`-typed field; the others are `string`). |

`CaseResult.FinalizeState()` rolls all of the above up into the case's
`State`: any non-empty `Err`, or any error in `CmdResults`, `MetricResults`,
`PodLogResults`, or `K8sResourceResults`, marks the case `CaseResultStateErrored`.
`NewCaseResult` and the `With*` builders call it for you.

### Building

The Makefile runs the Go toolchain inside the SUSE BCI golang image, so Docker
is the only hard prerequisite.

```bash
make go/build     # compile ./dist/hvperf
make go/test      # go test -cover -race -shuffle=on ./...
```

Useful overrides:

| Variable | Default | Purpose |
| --- | --- | --- |
| `IMAGE_PLATFORMS` | `linux/amd64` | Target platform for `go/build` and `tools` |
| `HARVESTER_VERSION` | `1.9` | Version prefix; `CLI_VERSION` becomes `<version>+<short-sha>` |
| `BCI_GO_TAG` | `1.26` | Tag of `registry.suse.com/bci/golang` used to build and test |
| `ETCD_VERSION` | `v3.6.14` | etcd version the `tools` target builds `etcdctl` and `benchmark` from |
| `DOCKER` | `docker` | Container runtime (e.g. `DOCKER=podman`) |

```bash
IMAGE_PLATFORMS=linux/arm64 make go/build
```

To build natively instead, Go 1.26+ works directly: `go build -o hvperf .`

## Repository layout

```
.
├── main.go                # entry point; blank-imports internal/suites to register built-ins
├── cmd/                   # cobra command tree (root, list, run, version) and k8s client setup
├── pkg/suites/            # public API: Suite interface, registry, options, results, marshalling
├── pkg/k8s/               # cluster helpers the suites share: namespaces, jobs, exec, logs, pod monitors
├── internal/suites/
│   ├── density/           # density suite
│   ├── etcd/              # etcd-benchmark suite
│   ├── nodes/             # node-capacity suite
│   ├── resourcefootprint/ # resource-footprint suite
│   ├── storage/           # storage-kbench suite
│   └── options/           # decodes the generic pkg/suites.Options into a suite's own options struct
├── METRICS.md             # the metrics the suites collect and how to read them
├── Dockerfile             # multi-stage: etcd tools + hvperf -> BCI base runtime
└── Makefile               # containerised build, test and tools targets
```

## Adding a suite

1. Create a package under `internal/suites/<name>/`.
2. Implement `pkg/suites.Suite`:

   ```go
   type MySuite struct {
       pkgsuites.SuiteMarshaler
       *pkgsuites.Clients
   }

   func (s *MySuite) Name() string        { return "my-suite" }
   func (s *MySuite) Description() string { return "what it measures" }
   func (s *MySuite) IsExclusive() bool   { return false }
   func (s *MySuite) RunE(ctx context.Context, runID, namespace string, opts pkgsuites.Options) pkgsuites.SuiteResult
   func (s *MySuite) SetClients(c *pkgsuites.Clients)
   ```

   Embedding `SuiteMarshaler` and setting `s.Marshal = s` at construction gives
   the suite its `list` table row and its JSON/YAML form for free.

   `runID` is generated once per `hvperf run` invocation and shared by every
   suite in it — use it to name and label anything the suite creates, and echo
   it back in the `SuiteResult`. `namespace` is where those resources belong.
   Record each check as a `CaseResult`; `Objects` is rendered as
   `(Kind) namespace/name` in the text output, and a case that could not be run
   for an environmental reason should set `State` to `CaseResultStateSkipped`
   instead of erroring. `NewCaseResult`/`NewCaseResultSkipped` build a
   `CaseResult` and finalize its `State` for you.

3. Give the suite an options struct. Settings shared across suites — the default
   namespace, job pod image and timeouts, the monitoring addon coordinates —
   live in `pkgsuites.DefaultGlobalOptions()`; decode them into your own struct
   with `internal/suites/options.FromOptions[*MyOptions]` and layer the
   suite-specific defaults on top. Passing that struct through
   `pkgsuites.ToSuiteParams` into `SuiteResult.Params` records the effective
   configuration in the results.

4. Register it from the package's `init()`:

   ```go
   func init() { pkgsuites.Register(NewMySuite()) }
   ```

5. Blank-import the package from `internal/suites/register.go` so `main` pulls
   it in.

Names are the registry key — two suites with the same `Name()` silently
overwrite each other. Suites should honour `ctx` cancellation and clean up
anything they create in the cluster.

## License

Apache 2.0 — see [LICENSE](LICENSE).
