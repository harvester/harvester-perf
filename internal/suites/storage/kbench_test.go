package storage

import (
	"reflect"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestKBenchOptionsDefaults(t *testing.T) {
	got, err := KBenchOptionsDefaults()
	if err != nil {
		t.Fatalf("KBenchOptionsDefaults() error = %v, want nil", err)
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
		JobPodContainerName:    "benchmark",
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

		PVCReadyTimeout:     300 * time.Second,
		PVCStorageClassName: "",
		PVCStorageSize:      resource.MustParse("33Gi"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("KBenchOptionsDefaults() = %+v, want %+v", got, want)
	}
}
