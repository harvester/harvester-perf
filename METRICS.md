# Metrics

The metrics that are used in the different test suites are listed below. The list
provides a brief description of the metric, why it is important, and what to watch
for when analyzing the metric.

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
