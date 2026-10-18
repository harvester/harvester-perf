package density

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/harvester/hvperf/pkg/resource"
	"github.com/harvester/hvperf/pkg/suites"
)

func TestDecodeOptionsOverridesProvidedFields(t *testing.T) {
	override := suites.Options{
		"BatchWaitTimeout": "90s",
		"MaxVMs":           1000,
		"VMI": suites.Options{
			"CPU": "200m",
		},
	}
	got, err := NewOptions(override)
	if err != nil {
		t.Fatalf("NewOptions() error = %v, want nil", err)
	}

	want := Options{
		BatchSize:             10,
		BatchWaitTimeout:      90 * time.Second,
		EtcdNamespace:         "kube-system",
		MaxVMs:                1000,
		NamespaceReadyTimeout: 300 * time.Second,
		VMI: resource.VMI{
			ContainerDisk: "quay.io/kubevirt/cirros-container-disk-demo:latest",
			Memory:        "90Mi",
			CPU:           "200m",
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("NewOptions() mismatch (-want +got):\n%s", diff)
	}
}
