package resource

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8swatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"

	kubevirtv1 "kubevirt.io/api/core/v1"
)

var VMGVR = schema.GroupVersionResource{
	Group:    "kubevirt.io",
	Version:  "v1",
	Resource: "virtualmachines",
}

var PVCGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}

var LonghornVolumeGVR = schema.GroupVersionResource{
	Group:    "longhorn.io",
	Version:  "v1beta2",
	Resource: "volumes",
}

const (
	longhornNamespace      = "longhorn-system"
	longhornFinalizerGrace = 30 * time.Minute
)

// DeleteRunVolumes deletes PVCs labelled with runID and explicitly removes the
// backing Longhorn volume objects. The explicit Longhorn delete prevents orphans
// when the CSI DeleteVolume call times out under load.
// Returns the Longhorn volume names so the caller can wait for them to disappear.
func DeleteRunVolumes(ctx context.Context, client dynamic.Interface, namespace, runID string) ([]string, error) {
	selector := metav1.ListOptions{LabelSelector: RunLabel + "=" + runID}

	// Collect PV names before deleting PVCs — Longhorn volume name == PV name.
	pvNames, err := collectPVNames(ctx, client, namespace, selector)
	if err != nil {
		return nil, err
	}

	if err := client.Resource(PVCGVR).Namespace(namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, selector); err != nil {
		return pvNames, fmt.Errorf("delete run PVCs: %w", err)
	}

	for _, name := range pvNames {
		if err := client.Resource(LonghornVolumeGVR).Namespace(longhornNamespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			slog.Error("DeleteRunVolumes: delete Longhorn volume failed", "volume", name, "err", err)
		}
	}
	return pvNames, nil
}

// WaitForDeletion polls until no resources matching selector exist in the given GVR/namespace,
// or ctx is cancelled. Poll interval is 5s.
func WaitForDeletion(ctx context.Context, client dynamic.Interface, gvr schema.GroupVersionResource, namespace string, selector metav1.ListOptions) error {
	for {
		list, err := client.Resource(gvr).Namespace(namespace).List(ctx, selector)
		if err != nil {
			return fmt.Errorf("list %s: %w", gvr.Resource, err)
		}
		if len(list.Items) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for %s deletion (%d remaining)", gvr.Resource, len(list.Items))
		case <-time.After(1 * time.Second):
		}
	}
}

// WaitLonghornVolumesByName polls until all named Longhorn volumes are gone.
// After 30 min, force-removes finalizers on stuck volumes so Kubernetes GC can proceed.
func WaitLonghornVolumesByName(ctx context.Context, client dynamic.Interface, names []string) error {
	grace := time.Now().Add(longhornFinalizerGrace)
	var patched bool
	for {
		var remaining []string
		for _, name := range names {
			_, err := client.Resource(LonghornVolumeGVR).Namespace(longhornNamespace).Get(ctx, name, metav1.GetOptions{})
			switch {
			case err == nil:
				remaining = append(remaining, name)
			case apierrors.IsNotFound(err):
				// Deleted.
			default:
				return fmt.Errorf("get Longhorn volume %s: %w", name, err)
			}
		}
		if len(remaining) == 0 {
			return nil
		}
		if !patched && time.Now().After(grace) {
			patch := []byte(`{"metadata":{"finalizers":[]}}`)
			for _, name := range remaining {
				if _, err := client.Resource(LonghornVolumeGVR).Namespace(longhornNamespace).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
					slog.Error("WaitLonghornVolumesByName: force-patch finalizers failed", "volume", name, "err", err)
				}
			}
			patched = true
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for Longhorn volumes (%d remaining)", len(remaining))
		case <-time.After(5 * time.Second):
		}
	}
}

func collectPVNames(ctx context.Context, client dynamic.Interface, namespace string, selector metav1.ListOptions) ([]string, error) {
	list, err := client.Resource(PVCGVR).Namespace(namespace).List(ctx, selector)
	if err != nil {
		return nil, fmt.Errorf("list run PVCs: %w", err)
	}
	names := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		pvName, _, _ := unstructured.NestedString(item.Object, "spec", "volumeName")
		if pvName != "" {
			names = append(names, pvName)
		}
	}
	return names, nil
}

// VM is everything needed to create one VirtualMachine: the identity the
// caller fills in per VM, plus the shape loaded from the density block of
// hvperf.yaml.
type VM struct {
	Namespace string `yaml:"-"`
	Name      string `yaml:"-"`
	RunID     string `yaml:"-"`
	ImageID   string `yaml:"-"`

	StorageClass string `yaml:"storageClass"`
	DiskSize     string `yaml:"diskSize"`
	Memory       string `yaml:"memory"`
	CPU          string `yaml:"cpu"`
}

// CreateAndWaitVM creates one VirtualMachine and waits for it to become ready.
func CreateAndWaitVM(ctx context.Context, client dynamic.Interface, vm VM) error {
	obj, err := render(vmTpl, vm)
	if err != nil {
		return err
	}

	lw := vmListWatch(ctx, client, vm.Namespace, vm.Name)
	if _, err := client.Resource(VMGVR).Namespace(vm.Namespace).Create(ctx, obj, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create VirtualMachine %s: %w", vm.Name, err)
	}
	if _, err := watchtools.UntilWithSync(ctx, lw, &unstructured.Unstructured{}, nil, vmCondition(vm.Name)); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return errors.Join(err, ctxErr)
		}
		return err
	}
	return nil
}

func vmListWatch(ctx context.Context, client dynamic.Interface, namespace, name string) *cache.ListWatch {
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = "metadata.name=" + name
			return client.Resource(VMGVR).Namespace(namespace).List(ctx, opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (k8swatch.Interface, error) {
			opts.FieldSelector = "metadata.name=" + name
			return client.Resource(VMGVR).Namespace(namespace).Watch(ctx, opts)
		},
	}
}

func vmCondition(name string) watchtools.ConditionFunc {
	return func(event k8swatch.Event) (bool, error) {
		obj, ok := event.Object.(*unstructured.Unstructured)
		if !ok || obj.GetName() != name {
			return false, nil
		}
		if event.Type == k8swatch.Deleted {
			return false, fmt.Errorf("VirtualMachine %s was deleted", name)
		}

		var vm kubevirtv1.VirtualMachine
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &vm); err != nil {
			return false, fmt.Errorf("convert VirtualMachine %s: %w", name, err)
		}
		return vm.Status.Ready, nil
	}
}
