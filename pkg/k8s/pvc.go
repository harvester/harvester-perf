package k8s

import (
	"context"
	"fmt"
	"time"

	"github.com/harvester/hvperf/pkg/suites"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	watchapi "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/watch"
)

// CreatePVC creates a Kubernetes PersistentVolumeClaim with the specified
// attributes. The spec of the PVC can be modified by providing one or more
// override functions that take a pointer to the PVC and modify it in place.
// It returns the created PVC, a cleanup function to delete the PVC, and an error
// if any occurred during the process. The call site is responsible for calling
// the cleanup function to delete the PVC when it is no longer needed.
func CreatePVC(
	ctx context.Context,
	c *suites.Clients,
	suiteName string,
	runID string,
	namespace string,
	storageClassName string,
	storageSize resource.Quantity,
	overrides ...func(*corev1.PersistentVolumeClaim) error,
) (*corev1.PersistentVolumeClaim, func() error, error) {
	namePrefix := fmt.Sprintf("%s-%s-", suiteName, runID)
	labelSelector := defaultLabelSelector(suiteName, runID)
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: namePrefix,
			Namespace:    namespace,
			Labels:       labelSelector.MatchLabels,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			VolumeMode: new(corev1.PersistentVolumeFilesystem),
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: storageSize,
				},
			},
		},
	}
	if storageClassName != "" {
		pvc.Spec.StorageClassName = &storageClassName
	}

	for _, mod := range overrides {
		if err := mod(pvc); err != nil {
			return nil, nil, err
		}
	}

	created, err := c.K8sClientSet.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return c.K8sClientSet.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, created.GetName(), metav1.DeleteOptions{})
	}

	created.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("PersistentVolumeClaim"))
	return created, cleanup, nil
}

// WaitPVCBound waits for the specified PersistentVolumeClaim to reach the "Bound" phase.
func WaitPVCBound(
	ctx context.Context,
	c *suites.Clients,
	pvc *corev1.PersistentVolumeClaim,
	boundTimeout time.Duration,
) (*corev1.PersistentVolumeClaim, error) {
	listWatch := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = fmt.Sprintf("metadata.name=%s", pvc.GetName())
			return c.K8sClientSet.CoreV1().PersistentVolumeClaims(pvc.GetNamespace()).List(ctx, options)
		},
		WatchFuncWithContext: func(ctx context.Context, options metav1.ListOptions) (watchapi.Interface, error) {
			options.FieldSelector = fmt.Sprintf("metadata.name=%s", pvc.GetName())
			return c.K8sClientSet.CoreV1().PersistentVolumeClaims(pvc.GetNamespace()).Watch(ctx, options)
		},
	}

	ctxWithTimeout, cancel := watch.ContextWithOptionalTimeout(ctx, boundTimeout)
	defer cancel()
	event, err := watch.UntilWithSync(ctxWithTimeout, listWatch, &corev1.PersistentVolumeClaim{}, nil, func(event watchapi.Event) (bool, error) {
		pvc, ok := event.Object.(*corev1.PersistentVolumeClaim)
		if !ok {
			return false, fmt.Errorf("unexpected object type: %T", event.Object)
		}
		return pvc.Status.Phase == corev1.ClaimBound, nil
	})
	if err != nil {
		return nil, fmt.Errorf("pvc %s not ready: %w", pvc.GetName(), err)
	}

	latest := event.Object.(*corev1.PersistentVolumeClaim)
	latest.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("PersistentVolumeClaim"))
	return latest, nil
}
