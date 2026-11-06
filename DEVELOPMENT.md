# Development

Everything you need to know about developing and contributing to this project.

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
│   ├── network/           # network-benchmark suite (netperf, iperf3)
│   ├── nodes/             # node-capacity suite
│   ├── resourcefootprint/ # control-plane-footprint suite
│   ├── storage/           # storage-kbench suite
│   └── options/           # decodes the generic pkg/suites.Options into a suite's own options struct
├── METRICS.md             # the metrics the suites collect and how to read them
├── Dockerfile             # multi-stage: etcd tools + hvperf -> BCI base runtime
└── Makefile               # containerised build, test and tools targets
```

## Building

The Makefile runs the Go toolchain inside the SUSE BCI golang image, so Docker
is the only hard prerequisite.

```bash
make go/build     # compile ./dist/hvperf
make go/test      # go test -cover -race -shuffle=on ./...
```

`go/test` builds with `--output=type=cacheonly` so no untagged image is left
behind. This needs BuildKit (docker buildx); it is untested with
`DOCKER=podman`.

Useful overrides:

| Variable | Default | Purpose |
| --- | --- | --- |
| `IMAGE_PLATFORMS` | `linux/amd64` | Target platform for `go/build`, `go/test` and `tools` |
| `HARVESTER_VERSION` | `1.9` | Version prefix; `CLI_VERSION` becomes `<version>+<short-sha>` |
| `BCI_GO_TAG` | `1.26` | Tag of `registry.suse.com/bci/golang` used to build and test |
| `ETCD_VERSION` | `v3.6.14` | etcd version the `tools` target builds `etcdctl` and `benchmark` from |
| `NETPERF_VERSION` | pinned commit | netperf commit the `tools` target builds `netperf` and `netserver` from |
| `IPERF3_VERSION` | `3.22` | iperf3 version the `tools` target builds |

```bash
IMAGE_PLATFORMS=linux/arm64 make go/build
```

To build natively instead, Go 1.26+ works directly: `go build -o hvperf .`

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
   func (s *MySuite) OptionsSchema() pkgsuites.Options { return options.DefaultMyOptions() }
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
   configuration in the results. The suite's `Default*Options()` is also its
   `OptionsSchema()`: user-supplied keys are validated against it (see
   [Config canonicalization](#config-canonicalization)).

4. Register it from the package's `init()`:

   ```go
   func init() { pkgsuites.Register(NewMySuite()) }
   ```

5. Blank-import the package from `internal/suites/register.go` so `main` pulls
   it in.

Names are the registry key — two suites with the same `Name()` silently
overwrite each other. Suites should honour `ctx` cancellation and clean up
anything they create in the cluster.

## Design Abstraction

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
| `control-plane-footprint` | no | Measures the cluster's resource footprint: per-namespace CPU/memory usage and requests, host memory, and node allocatable, via Prometheus queries. |
| `density` | yes | Import a VM image, ramp up VMs concurrently until failure, and record VMI boot-latency percentiles (p50/p95/p99). |
| `network-benchmark` | yes | Run netperf `TCP_STREAM`, `TCP_CRR`, `TCP_RR`, `UDP_STREAM` and `UDP_RR`, then iperf3 `TCP_STREAM` and `UDP_STREAM`, between a server and a client job pod (pod network, same or different nodes), recording throughput and latency. The `netperf`/`netserver`/`iperf3` binaries are copied into the pods from the tools directory. |
| `storage-kbench` | yes | Run Longhorn's `kbench` job against a fresh PVC to benchmark bandwidth, IOPS and latency, then read the matching Longhorn metrics from Prometheus. |

### Config canonicalization

`hvperf run` reads user options from a YAML file, keyed by suite name plus an
optional `global` section. Before any suite runs, `canonicalizeRunOptions`
(`cmd/run.go`) validates and normalizes those inputs. The `Default*Options()`
functions in `internal/suites/options/default.go` act as the **schema**:

- Each suite returns its defaults from `OptionsSchema()`. The `global` section
  is checked against `DefaultGlobalOptions()`.
- `Options.Canonicalize(schema)` (`pkg/suites/options.go`) matches each user
  key against the schema, ignoring case and separators (`-`, `_`, `.`), and
  rewrites it to the schema's spelling. `etcd-endpoints`, `etcdEndpoints` and
  `ETCDENDPOINTS` all become `etcdEndpoints`. Nested sections (such as the
  `vmi` section for `density`) are canonicalized recursively.
- The run fails fast, before touching the cluster, on: an unknown key, an
  empty key, two keys that collapse to the same schema key, a section for a
  suite that is not being run, and a nested/scalar shape mismatch against the
  schema.
- Only keys are checked. Value types are validated later, when
  `options.FromOptions` decodes the merged options into the suite's own struct.

At run time each suite receives `global` options first, then its own section
on top, then `toolsDir`, which cannot be overridden. A new option is therefore
only accepted from users once it has an entry in the suite's `Default*Options()`
— add it there, even if the default is zero or empty.

### The `config` command

`cmd/config.go` exposes the schema to users:

- `hvperf config template` dumps every suite's `OptionsSchema()` (suites with an
  empty schema are skipped) plus `DefaultGlobalOptions()` under `global`, as
  YAML on stdout. It is generated from the schema, so a new option shows up in
  the template as soon as it has a `Default*Options()` entry.
- `hvperf config validate -c <file>` loads the file with `loadRunOptions` and
  runs it through `canonicalizeRunOptions` against all registered suites. It
  never contacts the cluster. All errors are joined into one report.

`run --config` calls the same `canonicalizeRunOptions`, so `validate` and `run`
cannot disagree about what is valid.

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
