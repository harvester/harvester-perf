package suites

import "time"

// Options contains custom options for test suites. The keys are the names of the
// test suites, and the values are the options for each suite.
type Options map[string]any

// DefaultGlobalOptions returns the default options for the system test suite.
func DefaultGlobalOptions() *Options {
	return &Options{
		"DefaultNamespace":       "harvester-perf-system",
		"JobActiveDeadline":      3600 * time.Second,
		"JobKeepAlive":           true,
		"JobPodContainerName":    "benchmark",
		"JobPodImageName":        "registry.suse.com/bci/bci-base",
		"JobPodImageTag":         "latest",
		"JobPodTTLAfterFinished": 300 * time.Second,
		"JobPodReadyTimeout":     3600 * time.Second,
		"JobSuspend":             false,
		"PrometheusURL":          "http://rancher-monitoring-prometheus.cattle-monitoring-system:9090",
		"EtcdNamespace":          "kube-system",
		"MonitoringNamespace":    "cattle-monitoring-system",
		"MonitoringAddonName":    "rancher-monitoring",
	}
}
