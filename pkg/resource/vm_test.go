package resource

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8swatch "k8s.io/apimachinery/pkg/watch"
	dynfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

// The manifests are rendered at runtime, so a template or field slip only
// shows up here, not at compile time.
func TestVMRenderCarriesSpecAndRunLabel(t *testing.T) {
	u, err := render(vmTpl, VM{
		Namespace: "ns", Name: "run-vm-0", RunID: "run", ImageID: "ns/img",
		StorageClass: "longhorn-v2", DiskSize: "2Gi", Memory: "128Mi", CPU: "200m",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := u.GetLabels()[RunLabel]; got != "run" {
		t.Fatalf("run label = %q, want %q", got, "run")
	}
	vct := u.GetAnnotations()["harvesterhci.io/volumeClaimTemplates"]
	for _, want := range []string{"ns/img", "longhorn-v2", "2Gi", "run-vm-0-disk"} {
		if !strings.Contains(vct, want) {
			t.Fatalf("volumeClaimTemplates missing %q: %s", want, vct)
		}
	}
	mem, _, _ := unstructured.NestedString(u.Object, "spec", "template", "spec", "domain", "resources", "requests", "memory")
	if mem != "128Mi" {
		t.Fatalf("memory = %q, want 128Mi", mem)
	}
}

func TestCreateAndWaitVMReturnsContextDeadline(t *testing.T) {
	client := dynfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{VMGVR: "VirtualMachineList"})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := CreateAndWaitVM(ctx, client, VM{
		Namespace: "ns", Name: "run-vm-0", RunID: "run", ImageID: "ns/img",
		StorageClass: "longhorn", DiskSize: "2Gi", Memory: "128Mi", CPU: "200m",
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
}

func TestCollectPVNamesReturnsListError(t *testing.T) {
	want := errors.New("API unavailable")
	client := dynfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{PVCGVR: "PersistentVolumeClaimList"})
	client.PrependReactor("list", "persistentvolumeclaims", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, want
	})

	_, err := collectPVNames(context.Background(), client, "ns", metav1.ListOptions{})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestWaitLonghornVolumesByNameReturnsGetError(t *testing.T) {
	want := errors.New("API unavailable")
	client := dynfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{LonghornVolumeGVR: "VolumeList"})
	client.PrependReactor("get", "volumes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, want
	})

	err := WaitLonghornVolumesByName(context.Background(), client, []string{"pv-1"})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestVMImageRenderCarriesURL(t *testing.T) {
	u, err := render(vmImageTpl, VMImage{
		Namespace: "ns", Name: "run-image", RunID: "run", URL: "http://example/img.qcow2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if u.GetKind() != "VirtualMachineImage" {
		t.Fatalf("kind = %q", u.GetKind())
	}
	if got, _, _ := unstructured.NestedString(u.Object, "spec", "url"); got != "http://example/img.qcow2" {
		t.Fatalf("url = %q", got)
	}
}

// Unschedulable can be temporary while a PVC provisions, so only Ready ends
// the wait; the caller's context controls the timeout.
func TestVMConditionWaitsForUnschedulable(t *testing.T) {
	cond := vmCondition("run-vm-0")

	vm := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "run-vm-0"},
	}}
	for _, status := range []string{"ErrorUnschedulable", "Provisioning", "WaitingForVolumeBinding", "Starting"} {
		vm.Object["status"] = map[string]any{"printableStatus": status}
		done, err := cond(k8swatch.Event{Type: k8swatch.Modified, Object: vm})
		if done || err != nil {
			t.Fatalf("status %q must keep waiting, got done=%v err=%v", status, done, err)
		}
	}

	vm.Object["status"] = map[string]any{"printableStatus": "Running", "ready": true}
	if done, err := cond(k8swatch.Event{Type: k8swatch.Modified, Object: vm}); !done || err != nil {
		t.Fatalf("ready VM must finish the wait, got done=%v err=%v", done, err)
	}
}
