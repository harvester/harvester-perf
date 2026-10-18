package nodes

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/harvester/hvperf/pkg/suites"
)

func TestNewCapacityOptions(t *testing.T) {
	got, err := NewCapacityOptions(suites.Options{})
	if err != nil {
		t.Fatalf("NewCapacityOptions() error = %v, want nil", err)
	}

	want := &CapacityOptions{
		NamespaceReadyTimeout: 300 * time.Second,
		PodActiveDeadline:     3600 * time.Second,
		PodImageName:          "registry.suse.com/bci/bci-base",
		PodImageTag:           "latest",
		PodReadyTimeout:       3600 * time.Second,
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("NewCapacityOptions() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewCapacityOptionsOverride(t *testing.T) {
	want, err := NewCapacityOptions(nil)
	if err != nil {
		t.Fatal(err)
	}
	want.PodImageTag = "v1"
	want.PodReadyTimeout = 10 * time.Second
	want.NamespaceReadyTimeout = 20 * time.Second // global default overridden

	got, err := NewCapacityOptions(suites.Options{
		"PodImageTag":           "v1",
		"PodReadyTimeout":       10 * time.Second,
		"NamespaceReadyTimeout": 20 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewCapacityOptions() error = %v, want nil", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("NewCapacityOptions() mismatch (-want +got):\n%s", diff)
	}
}
