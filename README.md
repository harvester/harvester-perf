# harvester-perf

This repository contains a collection of tools, pipelines and documentation for
assessing Harvester performance and benchmark results.

`hvperf` is a CLI for running performance, capacity and benchmark test suites
against [SUSE Harvester](https://harvesterhci.io) clusters.

## Table of contents

- [Requirements](#requirements)
- [Concept](#concept)
- [Quick start](#quick-start)
- [Commands](#commands)
  - [Setting up the tools](#setting-up-the-tools)
  - [Configuration File For Parameters Overrides](#configuration-file-for-parameters-overrides)
- [Development](#development)
- [License](#license)

## Requirements

* A kubeconfig with cluster-admin on the target Harvester cluster.
* Docker (or a compatible runtime) for running build and test targets.
* Go 1.26+ only if you build outside the container.
* The `rancher-monitoring` addon, only for the metrics cases; suites skip those
cases when it is not enabled.
* For `etcd-benchmark`'s metrics cases, the RKE2 server config
  [`etcd-expose-metrics`](https://docs.rke2.io/reference/server_config#database)
  must be set to `true`; etcd does not expose metrics otherwise.
* For the `storage-kbench` suite, the pod requires 4 CPUs to effectively run the
  benchmark. If the cluster has fewer than 4 CPUs available, the pod will not start.

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
hvperf run network-benchmark
```

## Commands

| Command | Description |
| --- | --- |
| `hvperf list` | List registered suites. `-o` accepts `table` (default), `name`, `json`, `yaml`. |
| `hvperf run <suite>[,<suite>...]` | Run the named suites, given as a single comma-separated argument. An unknown name fails the run. An exclusive suite cannot be run together with other suites. `-o` accepts `text` (default), `json`, `yaml`. |
| `hvperf config template` / `validate -c <file>` | Print a config template holding every overridable parameter and its default, or check a config file without running anything. See [Configuration file](#configuration-file-for-parameters-overrides). |
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

Some suites need extra binaries: `etcd-benchmark` uses `etcdctl` and
`benchmark`, and `network-benchmark` uses `netperf`, `netserver` and `iperf3` (a
statically linked build, without OpenSSL). `hvperf run` reads them from
`~/.hvperf/tools`. Set that directory up with `hvperf tool`:

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
container; honours `ETCD_VERSION`, `NETPERF_VERSION` and `IPERF3_VERSION`).

The archive must be a `.tar.gz` of regular files (the tool binaries); other
entry types are skipped. Files are extracted flat into the tools directory,
overwriting existing ones.

`make tools` uses the Dockerfile to build `hvperf` and the tools that the suites
need. The etcd tools are built from the etcd source at the version pinned in the
Dockerfile (`ETCD_VERSION`, currently `v3.6.14`). The runtime base is
`registry.suse.com/bci/bci-base` at `BCI_TAG` (`16.0`).

### Configuration File For Parameters Overrides

Test suites parameters can be overridden via a configuration file, using the
`--config` flag.

```bash
hvperf run density --config ~/my-cluster.yaml
```

The file is YAML with one section per suite, plus an optional `global` section
for settings shared by all suites. Only the parameters you want to change need
to be listed; everything else keeps its default. Keys are matched
case-insensitively and may use `-`, `_` or `.` as separators, so `etcd-endpoints`
and `etcdEndpoints` are the same key. An unknown key or suite name is an error.

Use `hvperf config` to build and check the file:

```bash
# print a template with every parameter at its default value
hvperf config template > my-cluster.yaml

# edit it (keep only the parameters you want to override), then check it
hvperf config validate --config my-cluster.yaml
```

`validate` reports every problem it finds and exits non-zero, without touching
the cluster. `run --config` performs the same check before starting.

## Development

See [DEVELOPMENT.md](DEVELOPMENT.md) for information on building, testing and
additional development details.

## License

Apache 2.0 — see [LICENSE](LICENSE).
