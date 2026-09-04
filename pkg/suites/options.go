package suites

import "time"

const DefaultNamespace = "harvester-perf-system"

// Options contains custom options for test suites. The keys are the names of the
// test suites, and the values are the options for each suite.
type Options map[string]any

// DefaultGlobalOptions returns the default options for the system test suite.
func DefaultGlobalOptions() *Options {
	return &Options{
		"JobActiveDeadline":               3600 * time.Second,
		"JobPodContainerName":             "benchmark",
		"JobPodImageName":                 "registry.suse.com/bci/bci-base",
		"JobPodImageTag":                  "latest",
		"JobPodTTLAfterFinished":          300 * time.Second,
		"JobPodReadyTimeout":              3600 * time.Second,
		"JobSuspend":                      false,
		"EtcdNamespace":                   "kube-system",
		"MonitoringAddonName":             "rancher-monitoring",
		"MonitoringNamespace":             "cattle-monitoring-system",
		"MonitoringServiceURL":            "http://rancher-monitoring-prometheus.cattle-monitoring-system:9090",
		"MonitoringWaitPodMonitorTimeout": 300 * time.Second,
		"MonitoringScrapeInterval":        60 * time.Second,
	}
}
