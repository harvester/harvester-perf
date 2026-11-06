package options

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/api/resource"
)

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

		if diff := cmp.Diff(got, want); diff != "" {
			t.Errorf("FromOptions() mismatch (-want +got):\n%s", diff)
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
			PVCBoundTimeout     time.Duration
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
			PVCBoundTimeout:     300 * time.Second,
			PVCStorageClassName: "",
			PVCStorageSize:      "33Gi",
		}

		if diff := cmp.Diff(got, want); diff != "" {
			t.Errorf("FromOptions() mismatch (-want +got):\n%s", diff)
		}
	})
}
