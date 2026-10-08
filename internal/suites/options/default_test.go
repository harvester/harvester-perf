package options

import (
	"reflect"
	"testing"
	"time"

	"github.com/harvester/hvperf/pkg/suites"
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
		"IOPSIoDepth":         128,
		"IOPSNumJobs":         8,
		"LatencyBlockSize":    "4K",
		"LatencyIoDepth":      1,
		"LatencyNumJobs":      1,
		"Mode":                "full",
		"TestSize":            "30G",
		"JobPodImageName":     "longhornio/kbench",
		"JobPodImageTag":      "main-head",
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
			TestSize            string
			JobPodImageName     string
			JobPodImageTag      string
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
			IOPSIoDepth:         128,
			IOPSNumJobs:         8,
			LatencyBlockSize:    "4K",
			LatencyIoDepth:      1,
			LatencyNumJobs:      1,
			Mode:                "full",
			TestSize:            "30G",
			JobPodImageName:     "longhornio/kbench",
			JobPodImageTag:      "main-head",
			PVCReadyTimeout:     300 * time.Second,
			PVCStorageClassName: "",
			PVCStorageSize:      "33Gi",
		}
		if got != want {
			t.Errorf("FromOptions() = %+v, want %+v", got, want)
		}
	})
}
