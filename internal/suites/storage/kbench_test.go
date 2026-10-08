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
		IOPSIoDepth:        128,
		IOPSNumJobs:        8,
		LatencyBlockSize:   "4K",
		LatencyIoDepth:     1,
		LatencyNumJobs:     1,
		Mode:               ModeFull,
		RampTime:           60 * time.Second,
		RunTime:            120 * time.Second,
		TestSize:           "30G",

		JobActiveDeadline:      3600 * time.Second,
		JobPodContainerName:    "benchmark",
		JobPodImageName:        "longhornio/kbench",
		JobPodImageTag:         "main-head",
		JobPodTTLAfterFinished: 300 * time.Second,
		JobPodReadyTimeout:     3600 * time.Second,
		JobSuspend:             false,

		NamespaceReadyTimeout: 300 * time.Second,
		PVCReadyTimeout:       300 * time.Second,
		PVCStorageClassName:   "",
		PVCStorageSize:        resource.MustParse("33Gi"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("KBenchOptionsDefaults() = %+v, want %+v", got, want)
	}
}
