package storage

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/harvester/hvperf/pkg/suites"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestNewKBenchOptions(t *testing.T) {
	got, err := NewKBenchOptions(suites.Options{})
	if err != nil {
		t.Fatalf("NewKBenchOptions() error = %v, want nil", err)
	}

	want := &KBenchOptions{
		CPUIdleProf:        CPUIdleProfDisabled,
		BandwidthBlockSize: "128K",
		BandwidthIoDepth:   16,
		BandwidthNumJobs:   4,
		IOPSBlockSize:      "4K",
		IOPSIoDepth:        32,
		IOPSNumJobs:        4,
		LatencyBlockSize:   "4K",
		LatencyIoDepth:     1,
		LatencyNumJobs:     1,
		Mode:               ModeFull,
		RampTime:           60 * time.Second,
		TestSize:           "30G",

		JobActiveDeadline:      3600 * time.Second,
		JobPodTTLAfterFinished: 300 * time.Second,
		JobPodReadyTimeout:     3600 * time.Second,
		JobSuspend:             false,

		KBenchImageName:  "longhornio/kbench",
		KBenchImageTag:   "main-head",
		KBenchLimitCPU:   resource.MustParse("4"),
		KBenchLimitMem:   resource.MustParse("512Mi"),
		KBenchRequestCPU: resource.MustParse("4"),
		KBenchRequestMem: resource.MustParse("256Mi"),

		LonghornNamespace:     "longhorn-system",
		MonitoringAddonName:   "rancher-monitoring",
		MonitoringNamespace:   "cattle-monitoring-system",
		NamespaceReadyTimeout: 300 * time.Second,

		PVCBoundTimeout:     300 * time.Second,
		PVCStorageClassName: "",
		PVCStorageSize:      resource.MustParse("33Gi"),
	}

	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("NewKBenchOptions() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewKBenchOptionsOverride(t *testing.T) {
	want, err := NewKBenchOptions(nil)
	if err != nil {
		t.Fatal(err)
	}
	want.IOPSNumJobs = 8 // suite default overridden
	want.TestSize = "10G"
	want.PVCStorageSize = resource.MustParse("20Gi")
	want.JobPodReadyTimeout = 10 * time.Second // global default overridden

	got, err := NewKBenchOptions(suites.Options{
		"IOPSNumJobs":        8,
		"TestSize":           "10G",
		"PVCStorageSize":     "20Gi",
		"JobPodReadyTimeout": 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewKBenchOptions() error = %v, want nil", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("NewKBenchOptions() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewKBenchOptionsOverrideInvalid(t *testing.T) {
	tests := map[string]suites.Options{
		"test size not smaller than PVC size": {"TestSize": "40G"},
		"unparsable test size":                {"TestSize": "bogus"},
	}
	for name, override := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := NewKBenchOptions(override); err == nil {
				t.Error("NewKBenchOptions() error = nil, want error")
			}
		})
	}
}
