package density

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/harvester/hvperf/pkg/resource"
	pkgsuites "github.com/harvester/hvperf/pkg/suites"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// Keys absent from the file must keep their default, not zero out — and the
// nested vm/vmImage blocks must merge the same way.
func TestLoadOptionsMergesOverDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hvperf.yaml")
	cfg := "density:\n  perVMTimeout: 90s\n  vm:\n    storageClass: longhorn-v2\n"
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	o, err := loadOptions(path)
	if err != nil {
		t.Fatal(err)
	}
	def := defaultOptions()
	if o.PerVMTimeout != 90*time.Second {
		t.Fatalf("perVMTimeout = %s, want 90s", o.PerVMTimeout)
	}
	if o.Concurrency != def.Concurrency {
		t.Fatalf("concurrency = %d, want default %d", o.Concurrency, def.Concurrency)
	}
	if o.VM.StorageClass != "longhorn-v2" {
		t.Fatalf("storageClass = %q", o.VM.StorageClass)
	}
	// Siblings inside the overridden vm block must survive.
	if o.VM.Memory != def.VM.Memory {
		t.Fatalf("memory = %q, want default %q", o.VM.Memory, def.VM.Memory)
	}
	if o.VMImage.URL != def.VMImage.URL {
		t.Fatalf("vmImage.url = %q, want default", o.VMImage.URL)
	}
}

// The ramp has no VM cap, so the first VM that fails to come up has to stop it —
// otherwise a cluster that never refuses a VM outright would make it create them
// forever.
func TestCreateAndWaitVMsStopsOnFirstFailure(t *testing.T) {
	dyn := dynfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{resource.VMGVR: "VirtualMachineList"})
	k8s := k8sfake.NewClientset()

	s := &DensitySuite{Clients: &pkgsuites.Clients{DynClientSet: dyn, K8sClientSet: k8s}}
	o := defaultOptions()
	o.Concurrency = 2
	o.PerVMTimeout = 100 * time.Millisecond

	done := make(chan struct{})
	var rampEr error
	go func() {
		defer close(done)
		rampEr = s.createAndWaitVMs(context.Background(), o, "ns", "run", "ns/img")
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("uncapped ramp did not stop on saturation")
	}

	if rampEr == nil || !strings.Contains(rampEr.Error(), "stopped at VM") {
		t.Fatalf("want the failing VM named as the reason the ramp stopped, got %v", rampEr)
	}
	// A VM abandoned by its watch must still exist: the ramp must not delete or
	// discard it, because countReadyVMs is what decides the ceiling.
	vms, err := dyn.Resource(resource.VMGVR).Namespace("ns").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(vms.Items) == 0 {
		t.Fatal("VMs created during the ramp must be left in the cluster to be counted")
	}
}

func TestUnschedulableReasonReadsPodScheduledCondition(t *testing.T) {
	k8s := k8sfake.NewClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Labels: map[string]string{"vm.kubevirt.io/name": "run-vm-0"}},
		Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{
			Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: "Unschedulable", Message: "Insufficient memory",
		}}},
	})
	s := &DensitySuite{Clients: &pkgsuites.Clients{K8sClientSet: k8s}}

	if got := s.unschedulableReason(context.Background(), resource.VM{Name: "run-vm-0", Namespace: "ns"}); got != "Insufficient memory" {
		t.Fatalf("reason = %q, want Insufficient memory", got)
	}
}

func TestCleanupKeepsImageWhenPVCListFails(t *testing.T) {
	dyn := dynfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		resource.VMGVR:      "VirtualMachineList",
		resource.PVCGVR:     "PersistentVolumeClaimList",
		resource.VMImageGVR: "VirtualMachineImageList",
	})
	dyn.PrependReactor("list", "persistentvolumeclaims", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("API unavailable")
	})
	imageDeleted := false
	dyn.PrependReactor("delete-collection", "virtualmachineimages", func(k8stesting.Action) (bool, runtime.Object, error) {
		imageDeleted = true
		return true, nil, nil
	})

	s := &DensitySuite{Clients: &pkgsuites.Clients{DynClientSet: dyn}}
	s.cleanup("ns", "run")
	if imageDeleted {
		t.Fatal("cleanup deleted VMImage after PVC list failed")
	}
}

// countReadyVMs must count only VMs reporting status.ready, including ones whose
// per-VM watch had already given up.
func TestCountReadyVMsCountsOnlyReady(t *testing.T) {
	scheme := runtime.NewScheme()
	objs := []runtime.Object{
		vmWithReady("run-vm-0", true),
		vmWithReady("run-vm-1", false),
		vmWithReady("run-vm-2", true),
	}
	dyn := dynfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{resource.VMGVR: "VirtualMachineList"}, objs...)

	s := &DensitySuite{Clients: &pkgsuites.Clients{DynClientSet: dyn}}
	ready, err := s.countReadyVMs(context.Background(), "ns", "run")
	if err != nil {
		t.Fatal(err)
	}
	if ready != 2 {
		t.Fatalf("ready = %d, want 2", ready)
	}
}

func vmWithReady(name string, ready bool) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kubevirt.io/v1",
		"kind":       "VirtualMachine",
		"metadata": map[string]any{
			"name":      name,
			"namespace": "ns",
			"labels":    map[string]any{resource.RunLabel: "run"},
		},
		"status": map[string]any{"ready": ready},
	}}
}
