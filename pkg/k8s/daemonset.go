package k8s

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/harvester/hvperf/pkg/suites"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// EnsureDaemonSetReady creates a Kubernetes DaemonSet with the specified
// parameters and waits until the DaemonSet's pods are ready. By default, the pods
// sleep for the duration of activeDeadline. While the pods are still alive,
// caller can issue multiple commands to the pod.
// The spec of the DaemonSet can be modified by providing one or more override
// functions that take a pointer to the DaemonSet and modify it in place. This
// can be used to customize the DaemonSet's behavior, such as changing the
// command, adding environment variables, or modifying resource requests and
// limits.
func EnsureDaemonSetReady(
	ctx context.Context,
	c *suites.Clients,
	suiteName string,
	runID string,
	namespace,
	imageName string,
	activeDeadline time.Duration,
	waitTimeout time.Duration,
	overrides ...func(*appsv1.DaemonSet) error,
) (*appsv1.DaemonSet, []*corev1.Pod, func() error, error) {
	namePrefix := fmt.Sprintf("%s-%s-", suiteName, runID)
	labelSelector := defaultLabelSelector(suiteName, runID)
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: namePrefix,
			Namespace:    namespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: defaultLabelSelector(suiteName, runID),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labelSelector.MatchLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "hvperf",
							Image:           imageName,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/bin/bash", "-c", fmt.Sprintf("sleep %v", activeDeadline.Seconds())},
							SecurityContext: &corev1.SecurityContext{
								Privileged: new(true),
							},
						},
					},
					Tolerations: []corev1.Toleration{
						{
							Key:      "node-role.kubernetes.io/control-plane",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						},
						{
							Key:      "node-role.kubernetes.io/master",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						},
						{
							Key:      "node-role.kubernetes.io/etcd",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
					HostPID: true,
				},
			},
		},
	}

	// if any overrides are provided, apply them to the ds spec
	for _, mod := range overrides {
		if err := mod(ds); err != nil {
			return nil, nil, nil, err
		}
	}

	created, err := c.K8sClientSet.AppsV1().DaemonSets(namespace).Create(ctx, ds, metav1.CreateOptions{})
	if err != nil {
		return nil, nil, nil, err
	}
	cleanup := func() error {
		return c.K8sClientSet.AppsV1().DaemonSets(namespace).Delete(ctx, created.Name, metav1.DeleteOptions{})
	}
	created.SetGroupVersionKind(appsv1.SchemeGroupVersion.WithKind("DaemonSet"))

	var (
		waitErr error
		updated *appsv1.DaemonSet
	)
	if err := wait.PollUntilContextTimeout(ctx, time.Second*30, waitTimeout, true, func(ctx context.Context) (bool, error) {
		// keep polling until either all pods are ready or the timeout expires.
		// intermediate errors are ignored to keep the wait alive, to ensure wait
		// doesn't terminate prematurely due to transient create-related errors.
		// waitErr is used to capture the last error encountered during the wait, so
		// that it can be returned to the caller.
		waitErr = nil
		ds, err := c.K8sClientSet.AppsV1().DaemonSets(namespace).Get(ctx, created.Name, metav1.GetOptions{})
		if err != nil {
			waitErr = err
			return false, nil
		}
		updated = ds
		return updated.Status.ObservedGeneration >= updated.Generation && updated.Status.DesiredNumberScheduled == updated.Status.NumberReady, nil
	}); err != nil {
		return nil, nil, cleanup, errors.Join(fmt.Errorf("failed to wait for DaemonSet pods to be ready: %w", err), waitErr)
	}

	var podsWithGVK []*corev1.Pod
	pods, err := c.K8sClientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(labelSelector),
	})
	if err != nil {
		return nil, nil, cleanup, fmt.Errorf("failed to list DaemonSet pods: %w", err)
	}
	for _, pod := range pods.Items {
		pod.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Pod"))
		podsWithGVK = append(podsWithGVK, &pod)
	}

	return updated, podsWithGVK, cleanup, nil
}
