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
