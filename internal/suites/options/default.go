package options

import (
	"time"

	"github.com/harvester/hvperf/pkg/suites"
)

// DefaultGlobalOptions returns the default options for the system test suite.
func DefaultGlobalOptions() suites.Options {
	return suites.Options{
		"EtcdNamespace":                   "kube-system",
		"EtcdReadyTimeout":                300 * time.Second,
		"JobActiveDeadline":               3600 * time.Second,
		"JobPodImageName":                 "registry.suse.com/bci/bci-base",
		"JobPodImageTag":                  "latest",
		"JobPodNode":                      "",
		"JobPodTTLAfterFinished":          300 * time.Second,
		"JobPodReadyTimeout":              3600 * time.Second,
		"JobSuspend":                      false,
		"LonghornNamespace":               "longhorn-system",
		"MonitoringAddonName":             "rancher-monitoring",
		"MonitoringNamespace":             "cattle-monitoring-system",
		"MonitoringRangeDuration":         300 * time.Second,
		"MonitoringWaitPodMonitorTimeout": 600 * time.Second, // 2x the EtcdRangeDuration to allow for Prometheus to scrape etcd metrics
		"NamespaceReadyTimeout":           300 * time.Second, // accommodate for cleanup in back-to-back test runs
	}
}

// DefaultEtcdBenchmarkOptions returns the default options for the etcd benchmark
// test suite.
func DefaultEtcdBenchmarkOptions() suites.Options {
	return suites.Options{
		"EtcdctlOutputFormat":     "simple",
		"EtcdEndpoints":           "https://127.0.0.1:2379",
		"EtcdMetricsPath":         "/metrics",
		"EtcdMetricsPortName":     "metrics",
		"EtcdMetricsScheme":       "http",
		"EtcdRemoteCopyTargetDir": "/usr/local/bin/",
		"EtcdRemoteTLSCertDir":    "/host/rancher/rke2/server/tls/etcd",
		"MonitoringOutputFormat":  "promql",
		"CheckPerfLoadSize":       "s",
		"PutLoadSize":             10000,
		"PutKeySize":              8,
		"PutValSize":              256,
		"GRPCClientCount":         1,
		"GRPCConnCount":           1,
	}
}

// DefaultKBenchOptions returns the default options for the kbench test suite.
func DefaultKBenchOptions() suites.Options {
	return suites.Options{
		"CPUIdleProf":         "disabled",
		"BandwidthBlockSize":  "128K",
		"BandwidthIoDepth":    16,
		"BandwidthNumJobs":    4,
		"IOPSBlockSize":       "4K",
		"IOPSIoDepth":         32,
		"IOPSNumJobs":         4,
		"LatencyBlockSize":    "4K",
		"LatencyIoDepth":      1,
		"LatencyNumJobs":      1,
		"Mode":                "full",
		"RampTime":            60 * time.Second, // kbench ramp time use in the promql range; kbench doesn't support overriding ramp time
		"TestSize":            "30G",
		"KBenchImageName":     "longhornio/kbench",
		"KBenchImageTag":      "main-head",
		"KBenchLimitCPU":      "4",
		"KBenchLimitMem":      "512Mi",
		"KBenchRequestCPU":    "4",
		"KBenchRequestMem":    "256Mi",
		"PVCBoundTimeout":     300 * time.Second,
		"PVCStorageClassName": "", // use cluster default storage class
		"PVCStorageSize":      "33Gi",
	}
}

// DefaultNodeCapacityOptions returns the default options for the node capacity
// test suite.
func DefaultNodeCapacityOptions() suites.Options {
	return suites.Options{
		"PodActiveDeadline": 3600 * time.Second,
		"PodImageName":      "registry.suse.com/bci/bci-base",
		"PodImageTag":       "latest",
		"PodReadyTimeout":   3600 * time.Second,
	}
}

// DefaultVMDensityOptions returns the default options for the VM density test
// suite.
func DefaultVMDensityOptions() suites.Options {
	return suites.Options{
		"BatchSize":        10,
		"BatchWaitTimeout": 5 * time.Minute,
		"VMI": suites.Options{
			"ContainerDisk": "quay.io/kubevirt/cirros-container-disk-demo:latest",
			"Memory":        "90Mi",
			"CPU":           "100m",
		},
	}
}
