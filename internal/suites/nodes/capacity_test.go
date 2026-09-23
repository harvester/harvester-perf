package nodes

import (
	"reflect"
	"testing"
	"time"
)

func TestCapacityOptionsDefaults(t *testing.T) {
	got, err := CapacityOptionsDefaults()
	if err != nil {
		t.Fatalf("CapacityOptionsDefaults() error = %v, want nil", err)
	}

	want := &CapacityOptions{
		NamespaceReadyTimeout: 300 * time.Second,
		PodActiveDeadline:     3600 * time.Second,
		PodImageName:          "registry.suse.com/bci/bci-base",
		PodImageTag:           "latest",
		PodReadyTimeout:       3600 * time.Second,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CapacityOptionsDefaults() = %+v, want %+v", got, want)
	}
}
