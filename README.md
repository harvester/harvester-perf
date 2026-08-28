# harvester-perf

This repository contains a collection of tools, pipelines and documentation for
assessing Harvester performance and benchmark results.

`hvperf` is a CLI for running performance, capacity and benchmark test suites
against [SUSE Harvester](https://harvesterhci.io) clusters.

Suites are driven from your workstation using an ordinary kubeconfig. Nothing is
installed on the Harvester nodes; suites that need on-node access (such as the
etcd benchmark) schedule a short-lived helper pod and clean up after themselves.

> **Status:** early development. The suite registry, CLI and etcd job plumbing
> are in place; individual suites are still being filled in. See
> [Test suites](#test-suites) for what each one currently does.

## Quick start

```bash
# build ./bin/hvperf (runs the compiler inside a container, see Building)
make go/build

# what can be run
./bin/hvperf list

# client and cluster versions
./bin/hvperf version

# run a single suite
./bin/hvperf run node-capacity

# run several suites (one comma-separated argument, not a space-separated list)
./bin/hvperf run node-capacity,etcd-benchmark

# run every registered suite
./bin/hvperf run all
```

Every command that talks to a cluster accepts the standard kubectl connection
flags (`--kubeconfig`, `--context`, `--namespace`, `--server`, `--as`, …), so
targeting a specific cluster works the same way it does with `kubectl`:

```bash
./bin/hvperf run all --kubeconfig ~/.kube/harvester.yaml --context prod
```

Suites that create resources put them in the `harvester-system-perf` namespace,
creating the namespace if it does not exist. Pass `--namespace` to use a
different one.

The Dockerfile builds a container image with `hvperf` and the tools the suites
need to run in the cluster. For example, running the etcd suite from a locally
built binary rather than the image will fail unless `etcdctl` and `benchmark` are
present at `/usr/local/bin`. See [Container image](#container-image) for how to
build and run the image.

## Commands

| Command | Description |
| --- | --- |
| `hvperf list` | List registered suites. `-o` accepts `table` (default), `name`, `json`, `yaml`. |
| `hvperf run <suite>[,<suite>...]` | Run the named suites, given as a single comma-separated argument. Unknown names are ignored. `-o` accepts `text` (default), `json`, `yaml`. |
| `hvperf run all` | Run every registered suite, read-only and read-write alike. |
| `hvperf version [--client-only]` | Print the client version and, unless `--client-only` is set, the cluster's server version. |
| `hvperf report` | Placeholder — not implemented yet. |

Results are written to stdout; suite progress is logged to stderr, so
`hvperf run ... -o json > results.json` keeps the two streams separate. Pass
`-q`/`--quiet` to silence the progress logging, or `-v <level>` to raise it.
Results are printed even when a suite fails, so partial output is not lost.

## Test suites

Suites are either **read-only** — they only query the API server — or
**read-write**, meaning they create or modify cluster resources. The mode is
reported by `hvperf list`; it does not filter what `hvperf run all` runs, which
is every registered suite.

| Suite | Mode | What it does |
| --- | --- | --- |
| `node-capacity` | read-only | Assess node resource capacity. Registered and runnable, but the implementation is still a stub. |
| `etcd-benchmark` | read-write | Exercises the cluster's etcd from a privileged `hostNetwork` job pod. |

## Building

The Makefile runs the Go toolchain inside the SUSE BCI golang image, so Docker
is the only hard prerequisite. Host `GOCACHE`/`GOMODCACHE` are bind-mounted into
the build container and output is written back as your own user, so no `sudo
chown` dance afterwards.

```bash
make go/build     # compile ./bin/hvperf
make go/test      # go test -cover -race -shuffle=on ./...
make go/tidy      # go mod tidy
make clean        # remove ./bin
```

Useful overrides:

| Variable | Default | Purpose |
| --- | --- | --- |
| `GOOS` / `GOARCH` | `linux` / `amd64` | Cross-compilation target for `go/build` |
| `HARVESTER_VERSION` | `1.9` | Version prefix; `CLI_VERSION` becomes `<version>+<short-sha>` |
| `BUILD_IMAGE_TAG` | `1.26` | Tag of `registry.suse.com/bci/golang` used to build and test |
| `DOCKER` | `docker` | Container runtime (e.g. `DOCKER=podman`) |

```bash
GOOS=darwin GOARCH=arm64 make go/build
```

To build natively instead, Go 1.26+ works directly: `go build -o bin/hvperf .`

## Container image

The image bundles `hvperf` with the tools the suites ship into the cluster:
upstream `etcdctl` and `benchmark`, built from the etcd source at the version
pinned in the Dockerfile (`ETCD_VERSION`, currently `v3.6.14`).

```bash
make image/build                                    # -> hvperf:<version>-<sha>
make image/run                                      # show version
make image/run IMAGE_RUN_ARGS="list"                # mounts ~/.kube read-only
make image/run IMAGE_RUN_ARGS="run etcd-benchmark"
make image/run IMAGE_RUN_ARGS="run all"
```

Overrides: `IMAGE_NAME`, `IMAGE_TAG`, `IMAGE_PLATFORMS` (default
`linux/amd64`), `IMAGE_OUTPUT_TYPE`.

## Repository layout

```
.
├── main.go                # entry point; blank-imports internal/suites to register built-ins
├── cmd/                   # cobra command tree (root, list, run, report, version) and k8s client setup
├── pkg/suites/            # public API: Suite interface, registry, options, results, marshalling
├── pkg/k8s/               # cluster helpers the suites share: namespaces, jobs, exec, logs
├── internal/suites/
│   ├── etcd/              # etcd-benchmark suite
│   ├── nodes/             # node-capacity suite
│   └── options/           # decodes the generic pkg/suites.Options into a suite's own options struct
├── poc/                   # shell-based prototype this CLI is being ported from
├── Dockerfile             # multi-stage: etcd tools + hvperf -> BCI base runtime
└── Makefile               # containerised build, test and image targets
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
   func (s *MySuite) IsReadWrite() bool   { return false }
   func (s *MySuite) RunE(ctx context.Context, runID, namespace string, opts pkgsuites.Options) (pkgsuites.SuiteResult, error)
   func (s *MySuite) SetClients(c *pkgsuites.Clients)
   ```

   Embedding `SuiteMarshaler` and setting `s.Marshal = s` at construction gives
   the suite its `list` table row and its JSON/YAML form for free.

   `runID` is generated once per `hvperf run` invocation and shared by every
   suite in it — use it to name and label anything the suite creates, and echo
   it back in the `SuiteResult`. `namespace` is where those resources belong.
   Record each check as a `CaseResult`; `Objects` is rendered as
   `(Kind) namespace/name` in the text output.

3. Register it from the package's `init()`:

   ```go
   func init() { pkgsuites.Register(NewMySuite()) }
   ```

4. Blank-import the package from `internal/suites/register.go` so `main` pulls
   it in.

Names are the registry key — two suites with the same `Name()` silently
overwrite each other. Suites should honour `ctx` cancellation and clean up
anything they create in the cluster.

## Proof of concept

`poc/` holds the shell implementation that preceded this CLI: a container that
runs `cluster-info`, `node-capacity`, `etcd-benchmark`, `controlplane-resources`
and `vm-density` suites and renders a markdown report. It covers more ground
than the Go CLI does today and is kept as a reference for the suites still to be
ported. See [`poc/README.md`](poc/README.md).

## Requirements

- A kubeconfig with cluster-admin on the target Harvester cluster.
- Docker (or a compatible runtime) for the build and image targets.
- Go 1.26+ only if you build outside the container.

## License

Apache 2.0 — see [LICENSE](LICENSE).
