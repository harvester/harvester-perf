package options

import (
	"time"

	"github.com/harvester/hvperf/pkg/suites"
)

// DefaultGlobalOptions returns the default options for the system test suite.
func DefaultGlobalOptions() suites.Options {
	return suites.Options{
		"etcdNamespace":                   "kube-system",
		"etcdReadyTimeout":                300 * time.Second,
		"jobActiveDeadline":               3600 * time.Second,
		"jobPodImageName":                 "registry.suse.com/bci/bci-base",
		"jobPodImageTag":                  "latest",
		"jobPodNode":                      "",
		"jobPodTTLAfterFinished":          300 * time.Second,
		"jobPodReadyTimeout":              3600 * time.Second,
		"jobSuspend":                      false,
		"longhornNamespace":               "longhorn-system",
		"monitoringAddonName":             "rancher-monitoring",
		"monitoringNamespace":             "cattle-monitoring-system",
		"monitoringRangeDuration":         300 * time.Second,
		"monitoringWaitPodMonitorTimeout": 600 * time.Second, // 2x the EtcdRangeDuration to allow for Prometheus to scrape etcd metrics
		"namespaceReadyTimeout":           300 * time.Second, // accommodate for cleanup in back-to-back test runs
	}
}

// DefaultEtcdBenchmarkOptions returns the default options for the etcd benchmark
// test suite.
func DefaultEtcdBenchmarkOptions() suites.Options {
	return suites.Options{
		"etcdctlOutputFormat":     "simple",
		"etcdEndpoints":           "https://127.0.0.1:2379",
		"etcdMetricsPath":         "/metrics",
		"etcdMetricsPortName":     "metrics",
		"etcdMetricsScheme":       "http",
		"etcdRemoteCopyTargetDir": "/usr/local/bin/",
		"etcdRemoteTLSCertDir":    "/host/rancher/rke2/server/tls/etcd",
		"monitoringOutputFormat":  "promql",
		"checkPerfLoadSize":       "s",
		"putLoadSize":             10000,
		"putKeySize":              8,
		"putValSize":              256,
		"grpcClientCount":         1,
		"grpcConnCount":           1,
	}
}

// DefaultKBenchOptions returns the default options for the kbench test suite.
func DefaultKBenchOptions() suites.Options {
	return suites.Options{
		"cpuIdleProf":         "disabled",
		"bandwidthBlockSize":  "128K",
		"bandwidthIoDepth":    16,
		"bandwidthNumJobs":    4,
		"iopsBlockSize":       "4K",
		"iopsIoDepth":         32,
		"iopsNumJobs":         4,
		"latencyBlockSize":    "4K",
		"latencyIoDepth":      1,
		"latencyNumJobs":      1,
		"mode":                "full",
		"rampTime":            60 * time.Second, // kbench ramp time use in the promql range; kbench doesn't support overriding ramp time
		"testSize":            "30G",
		"kbenchImageName":     "longhornio/kbench",
		"kbenchImageTag":      "main-head",
		"kbenchLimitCPU":      "4",
		"kbenchLimitMem":      "512Mi",
		"kbenchRequestCPU":    "4",
		"kbenchRequestMem":    "256Mi",
		"pvcBoundTimeout":     300 * time.Second,
		"pvcStorageClassName": "", // use cluster default storage class
		"pvcStorageSize":      "33Gi",
	}
}

// DefaultNodeCapacityOptions returns the default options for the node capacity
// test suite.
func DefaultNodeCapacityOptions() suites.Options {
	return suites.Options{
		"podActiveDeadline": 3600 * time.Second,
		"podImageName":      "registry.suse.com/bci/bci-base",
		"podImageTag":       "latest",
		"podReadyTimeout":   3600 * time.Second,
	}
}

// DefaultVMDensityOptions returns the default options for the VM density test
// suite.
func DefaultVMDensityOptions() suites.Options {
	return suites.Options{
		"batchSize":        10,
		"batchWaitTimeout": 5 * time.Minute,
		"vmi": suites.Options{
			"containerDisk": "quay.io/kubevirt/cirros-container-disk-demo:latest",
			"memory":        "90Mi",
			"cpu":           "100m",
		},
	}
}

// DefaultNetworkOptions returns the default options for the netperf benchmark
// test suite.
func DefaultNetworkOptions() suites.Options {
	return suites.Options{
		"iPerfClientMessageSizeBytes": 1024,
		"iPerfClientParallelStreams":  1,
		"iPerfIPFamily":               "-4",
		"iPerfServerPort":             22865,
		"iPerfTestDurationSeconds":    10,
		"iPerfRemoteCopyTargetDir":    "/usr/local/bin/",
		"netPerfIPFamily":             "-4",
		"netPerfRemoteCopyTargetDir":  "/usr/local/bin/",
		"netPerfTestStartWaitSeconds": 2,
		"netPerfTestLengthSeconds":    10,
	}
}
