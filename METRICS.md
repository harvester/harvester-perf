# Metrics

The metrics that are used in the different test suites are listed below. The list
provides a brief description of the metric, why it is important, and what to watch
for when analyzing the metric.

## Density

Metrics | Why | Watch For
------- | --- | ---------
`kubevirt_vmi_phase_transition_time_from_creation_seconds_bucket{phase="Running"}` (histogram) | Time from VMI creation to `Running` phase — the primary VM boot latency signal | p50, p95, and p99 are reported; p99 climbing relative to p50 indicates tail-latency pressure under load; compare across runs at the same concurrency level

## Etcd Benchmark

Metrics | Why | Watch For
------- | --- | ---------
`etcd_disk_wal_fsync_duration_seconds` (histogram) | Time to fsync the WAL — the single most cited etcd health signal | p99 should stay under ~10ms; this is etcd's own commonly-cited threshold, not an arbitrary number
`etcd_disk_backend_commit_duration_seconds` (histogram) | Time to commit changes to the bbolt backend store | Watch for sudden relative jumps (e.g. >25% increase over 5 min) more than a fixed absolute number
`etcd_disk_wal_write_bytes_total` (counter) | Raw WAL write throughput | `rate()` of this gives you actual disk write bandwidth consumed
`etcd_network_peer_round_trip_time_seconds` (histogram, per peer) | RTT between etcd members — directly gates how fast Raft consensus rounds complete | High p99 RTT causes heartbeat misses → disruptive leader elections, independent of disk health
`etcd_network_peer_sent_failures_total` / `peer_received_failures_total` | Peer connection failures | Rising counts indicate network instability between members

## Resource Footprint

Metrics | Why | Watch For
------- | --- | ---------
`container_cpu_usage_seconds_total` | Per-namespace, per-node idle CPU usage of Harvester control-plane containers | Higher idle usage across releases may signal increased background activity; cross-reference with `kube_pod_container_resource_requests` to confirm footprint growth
`container_memory_working_set_bytes` | Per-namespace, per-node working-set memory of Harvester control-plane containers | Higher idle memory usage across releases may signal increased background consumption; cross-reference with `kube_pod_container_resource_requests` to confirm footprint growth
`kube_pod_container_resource_requests` | CPU and memory reserved by Harvester control-plane pods, per namespace and node | An increase across releases is the primary footprint regression signal — requests are what the scheduler locks away from users, regardless of actual usage
`kube_node_status_allocatable` | CPU and memory available for user workloads after node-level reservations | A decrease across releases means users have less schedulable capacity — the direct user-visible impact of footprint growth
`node_memory_MemTotal_bytes` / `node_memory_MemAvailable_bytes` | Host memory capacity and available memory | A sustained drop in `MemAvailable` across releases without a change in `MemTotal` indicates the control plane is consuming more host memory

## Storage

Metrics | Why | Watch For
------- | --- | ---------
`longhorn_volume_robustness{state="healthy"}` | Confirm every replica was present and in sync for the whole run | `1` for the whole run. Any sample below `1` during the run means the volume was unhealthy due to either missing replica, replicas rebuild, unhealthy volume etc.
`longhorn_volume_state{state="attached"}` | Confirm whether the volume stayed attached to the same node throughout | Any sample below `1`. More than one `node` label value in the window means a detach and reattach happened, which implies the engine restarted, so `kbench` might see I/O errors or stalls.
`longhorn_instance_manager_cpu_usage_millicpu` | CPU used by the Longhorn datapath (engine and replica) processes on each node. This is the cost invisible to `kbench` and is spread across nodes holding a replica | Look for spikes in CPU usage that correlate with `kbench` I/O latency spikes.
`longhorn_instance_manager_memory_usage_bytes` | Memory footprint of Longhorn datapath on each node. It reports how Longhorn's memory cost scales with the increase in number of volumes | Memory that keeps growing during the run, doesn't return to baseline afterwards, large spikes above the baseline, uneven usage across nodes.

### Understanding Kbench Results

Ref: <https://github.com/longhorn/kbench#understanding-the-result>

IOPS: IO operations per second. Higher is better. It's a measurement of how many
IO operations can the device handle in a second, mainly concerning the smaller IO
chunks, e.g. 4k.

Bandwidth: Also called Throughput. Higher is better. It's a measurement of how
many data can the device read/write in a second. It's mainly concering the bigger
IO chunks, e.g. 128k.

Latency: The total time each request spent in the IO path. Lower is better. It's
a measurement of how efficient is the storage system to handle each request. The
data path overhead of a storage system can be expressed as the latency it added
on top of the native storage system (SSD/NVMe).

## Network

### Understanding NetPerf Results

Ref: <https://hewlettpackard.github.io/netperf/doc/netperf.html>

#### Control and Data Connections

When you execute netperf it will establish a “control connection” to the remote
system. This connection will be used to pass test configuration information and
results to and from the remote system. Regardless of the type of test to be run,
the control connection will be a TCP connection using BSD sockets. The control
connection can use either IPv4 or IPv6.

Once the control connection is up and the configuration information has been
passed, a separate “data” connection will be opened for the measurement itself
using the API's and protocols appropriate for the specified test. When the test
is completed, the data connection will be torn-down and results from the
netserver will be passed-back via the control connection and combined with
netperf's result for display to the user.

Netperf places no traffic on the control connection while a test is in progress.
Certain TCP options, such as `SO_KEEPALIVE`, if set as your systems' default, may
put packets out on the control connection while a test is in progress. Generally
speaking this will have no effect on the results.

#### CPU Utilization

CPU utilization in netperf is reported as a value between 0 and 100% regardless
of the number of CPUs involved. In addition to CPU utilization, netperf will
report a metric called a service demand. The service demand is the normalization
of CPU utilization and work performed. For a _STREAM test it is the microseconds
of CPU time consumed to transfer on KB (K == 1024) of data. For a_RR test it is
the microseconds of CPU time consumed processing a single transaction. For both
CPU utilization and service demand, lower is better.

Service demand can be particularly useful when trying to gauge the effect of a
performance change. It is essentially a measure of efficiency, with smaller value
s being more efficient and thus “better.”

#### Test Profiles

Tests are named with certain suffixes that indicate the type of test being run.

##### Stream Tests

The `_STREAM` tests are used to assess bulk or unidirectional transfer performance.
Everyone wants to know how many bits or bytes per second they can push across the
network.

`TCP_STREAM` - the default test in netperf. It is quite simple, transferring
some quantity of data from the system running netperf to the system running
netserver. While time spent establishing the connection is not included in the
throughput calculation, time spent flushing the last of the data to the remote
at the end of the test is. This is how netperf knows that all the data it sent
was received by the remote.

`UDP_STREAM` - similar to a `TCP_STREAM` test except UDP is used as the transport
rather than TCP. A UDP_STREAM test has no end-to-end flow control - UDP provides
none and neither does netperf. The biggest of these implications is the data
which is sent might not be received by the remote. For this reason, the output of
a UDP_STREAM test shows both the sending and receiving throughput. On some
platforms, it may be possible for the sending throughput to be reported as a
value greater than the maximum rate of the link. This is common when the CPU(s)
are faster than the network and there is no intra-stack flow-control.

##### Request/Response Tests

Tests with the `_RR` suffix are used to assess request/response or transaction
performance.

A `TCP_RR` test can be thought-of as a user-space to user-space ping with no
think time - it is by default a synchronous, one transaction at a time,
request/response test. The transaction rate is the number of complete
transactions exchanged divided by the length of time it took to perform those
transactions.

If the two Systems Under Test are otherwise identical, a `TCP_RR` test with the
same request and response size should be symmetric - it should not matter which
way the test is run, and the CPU utilization measured should be virtually the
same on each system. If not, it suggests that the CPU utilization mechanism being
used may have some, well, issues measuring CPU utilization completely and accurately.

Time to establish the TCP connection is not counted in the result.

The `UDP_RR` is very much the same as a TCP_RR test except UDP is used rather
than TCP. UDP does not provide for retransmission of lost UDP datagrams, and
netperf does not add anything for that either. This means that if _any_ request
or response is lost, the exchange of requests and responses will stop from that
point until the test timer expires. Netperf will not really “know” this has
happened - the only symptom will be a low transaction per second rate.

A `TCP_CC` test simply measures how fast the pair of systems can open and close
connections between one another in a synchronous (one at a time) manner. While
this is considered an `_RR` test, no request or response is exchanged over the connection.

A `TCP_CRR` test is like a merger of a TCP_RR and TCP_CC test which measures the
performance of establishing a connection, exchanging a single request/response
transaction, and tearing-down that connection. This is very much like what happens
in an HTTP 1.0 or HTTP 1.1 connection when HTTP Keepalives are not used. In fact,
the TCP_CRR test was added to netperf to simulate just that.
