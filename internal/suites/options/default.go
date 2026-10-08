package options

import (
	"time"

	"github.com/harvester/hvperf/pkg/suites"
)

// DefaultGlobalOptions returns the default options for the system test suite.
func DefaultGlobalOptions() *suites.Options {
	return &suites.Options{
		"EtcdNamespace":                   "kube-system",
		"EtcdReadyTimeout":                300 * time.Second,
		"JobActiveDeadline":               3600 * time.Second,
		"JobPodContainerName":             "benchmark",
		"JobPodImageName":                 "registry.suse.com/bci/bci-base",
		"JobPodImageTag":                  "latest",
		"JobPodTTLAfterFinished":          300 * time.Second,
		"JobPodReadyTimeout":              3600 * time.Second,
		"JobSuspend":                      false,
		"MonitoringAddonName":             "rancher-monitoring",
		"MonitoringNamespace":             "cattle-monitoring-system",
		"MonitoringRangeDuration":         300 * time.Second,
		"MonitoringWaitPodMonitorTimeout": 600 * time.Second, // 2x the EtcdRangeDuration to allow for Prometheus to scrape etcd metrics
		"NamespaceReadyTimeout":           300 * time.Second, // accommodate for cleanup in back-to-back test runs
	}
}

// DefaultNodeCapacityOptions returns the default options for the node capacity
// test suite.
func DefaultNodeCapacityOptions() *suites.Options {
	return &suites.Options{
		"PodActiveDeadline": 3600 * time.Second,
		"PodImageName":      "registry.suse.com/bci/bci-base",
		"PodImageTag":       "latest",
		"PodReadyTimeout":   3600 * time.Second,
	}
}

// DefaultKBenchOptions returns the default options for the kbench test suite.
func DefaultKBenchOptions() *suites.Options {
	return &suites.Options{
		"CPUIdleProf":         "disabled",
		"BandwidthBlockSize":  "128K",
		"BandwidthIoDepth":    16,
		"BandwidthNumJobs":    4,
		"IOPSBlockSize":       "4K",
		"IOPSIoDepth":         128,
		"IOPSNumJobs":         8,
		"LatencyBlockSize":    "4K",
		"LatencyIoDepth":      1,
		"LatencyNumJobs":      1,
		"Mode":                "full",
		"RampTime":            60 * time.Second,
		"RunTime":             120 * time.Second,
		"TestSize":            "30G",
		"JobPodImageName":     "longhornio/kbench",
		"JobPodImageTag":      "main-head",
		"PVCReadyTimeout":     300 * time.Second,
		"PVCStorageClassName": "", // use cluster default storage class
		"PVCStorageSize":      "33Gi",
	}
}
