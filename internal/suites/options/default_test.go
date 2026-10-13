package options

import (
	"reflect"
	"testing"
	"time"

	"github.com/harvester/hvperf/pkg/suites"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestDefaultGlobalOptions(t *testing.T) {
	want := &suites.Options{
		"EtcdNamespace":                   "kube-system",
		"EtcdReadyTimeout":                300 * time.Second,
		"JobActiveDeadline":               3600 * time.Second,
		"JobPodContainerName":             "benchmark",
		"JobPodImageName":                 "registry.suse.com/bci/bci-base",
		"JobPodImageTag":                  "latest",
		"JobPodTTLAfterFinished":          300 * time.Second,
		"JobPodReadyTimeout":              3600 * time.Second,
		"JobSuspend":                      false,
		"LonghornNamespace":               "longhorn-system",
		"MonitoringAddonName":             "rancher-monitoring",
		"MonitoringNamespace":             "cattle-monitoring-system",
		"MonitoringRangeDuration":         300 * time.Second,
		"MonitoringWaitPodMonitorTimeout": 600 * time.Second,
		"NamespaceReadyTimeout":           300 * time.Second,
	}

	if got := DefaultGlobalOptions(); !reflect.DeepEqual(got, want) {
		t.Errorf("DefaultGlobalOptions() = %+v, want %+v", got, want)
	}
}

func TestDefaultNodeCapacityOptions(t *testing.T) {
	want := &suites.Options{
		"PodActiveDeadline": 3600 * time.Second,
		"PodImageName":      "registry.suse.com/bci/bci-base",
		"PodImageTag":       "latest",
		"PodReadyTimeout":   3600 * time.Second,
	}

	if got := DefaultNodeCapacityOptions(); !reflect.DeepEqual(got, want) {
		t.Errorf("DefaultNodeCapacityOptions() = %+v, want %+v", got, want)
	}
}

func TestDefaultKBenchOptions(t *testing.T) {
	want := &suites.Options{
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
		"RampTime":            60 * time.Second,
		"TestSize":            "30G",
		"KBenchImageName":     "longhornio/kbench",
		"KBenchImageTag":      "main-head",
		"KBenchLimitCPU":      "4",
		"KBenchLimitMem":      "512Mi",
		"KBenchRequestCPU":    "4",
		"KBenchRequestMem":    "256Mi",
		"PVCReadyTimeout":     300 * time.Second,
		"PVCStorageClassName": "",
		"PVCStorageSize":      "33Gi",
	}

	if got := DefaultKBenchOptions(); !reflect.DeepEqual(got, want) {
		t.Errorf("DefaultKBenchOptions() = %+v, want %+v", got, want)
	}
}

// TestDefaultOptionsFromOptions checks that each defaults function produces
// an Options map that round-trips through FromOptions into its corresponding
// typed options struct, since that's how suites consume these defaults.
func TestDefaultOptionsFromOptions(t *testing.T) {
	t.Run("node capacity", func(t *testing.T) {
		type nodeCapacityOptions struct {
			PodActiveDeadline time.Duration
			PodImageName      string
			PodImageTag       string
			PodReadyTimeout   time.Duration
		}

		got, err := FromOptions[nodeCapacityOptions](DefaultNodeCapacityOptions())
		if err != nil {
			t.Fatalf("FromOptions() error = %v, want nil", err)
		}
		want := nodeCapacityOptions{
			PodActiveDeadline: 3600 * time.Second,
			PodImageName:      "registry.suse.com/bci/bci-base",
			PodImageTag:       "latest",
			PodReadyTimeout:   3600 * time.Second,
		}
		if got != want {
			t.Errorf("FromOptions() = %+v, want %+v", got, want)
		}
	})

	t.Run("kbench", func(t *testing.T) {
		type kBenchOptions struct {
			CPUIdleProf         string
			BandwidthBlockSize  string
			BandwidthIoDepth    int
			BandwidthNumJobs    int
			IOPSBlockSize       string
			IOPSIoDepth         int
			IOPSNumJobs         int
			LatencyBlockSize    string
			LatencyIoDepth      int
			LatencyNumJobs      int
			Mode                string
			RampTime            time.Duration
			TestSize            string
			KBenchImageName     string
			KBenchImageTag      string
			KBenchLimitCPU      resource.Quantity
			KBenchLimitMem      resource.Quantity
			KBenchRequestCPU    resource.Quantity
			KBenchRequestMem    resource.Quantity
			PVCReadyTimeout     time.Duration
			PVCStorageClassName string
			PVCStorageSize      string
		}

		got, err := FromOptions[kBenchOptions](DefaultKBenchOptions())
		if err != nil {
			t.Fatalf("FromOptions() error = %v, want nil", err)
		}
		want := kBenchOptions{
			CPUIdleProf:         "disabled",
			BandwidthBlockSize:  "128K",
			BandwidthIoDepth:    16,
			BandwidthNumJobs:    4,
			IOPSBlockSize:       "4K",
			IOPSIoDepth:         32,
			IOPSNumJobs:         4,
			LatencyBlockSize:    "4K",
			LatencyIoDepth:      1,
			LatencyNumJobs:      1,
			Mode:                "full",
			RampTime:            60 * time.Second,
			TestSize:            "30G",
			KBenchImageName:     "longhornio/kbench",
			KBenchImageTag:      "main-head",
			KBenchLimitCPU:      resource.MustParse("4"),
			KBenchLimitMem:      resource.MustParse("512Mi"),
			KBenchRequestCPU:    resource.MustParse("4"),
			KBenchRequestMem:    resource.MustParse("256Mi"),
			PVCReadyTimeout:     300 * time.Second,
			PVCStorageClassName: "",
			PVCStorageSize:      "33Gi",
		}
		if got != want {
			t.Errorf("FromOptions() = %+v, want %+v", got, want)
		}
	})
}
