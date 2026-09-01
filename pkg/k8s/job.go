package k8s

import (
	"context"
	"fmt"
	"time"

	"github.com/harvester/hvperf/pkg/suites"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	watchapi "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/watch"
)

// EnsureJobReady creates a Kubernetes Job with the specified parameters and waits until the Job's pod is ready.
func EnsureJobReady(
	ctx context.Context,
	c *suites.Clients,
	suiteName string,
	runID string,
	namespace string,
	node string,
	imageName string,
	jobActiveDeadline time.Duration,
	readyTimeout time.Duration,
	ttlAfterFinished time.Duration,
	suspend bool,
	overrrides ...func(*batchv1.Job) error,
) (*batchv1.Job, *corev1.Pod, error) {
	namePrefix := fmt.Sprintf("%s-%s-", suiteName, runID)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: namePrefix,
			Namespace:    namespace,
		},
		Spec: batchv1.JobSpec{
			ActiveDeadlineSeconds:   new(int64(jobActiveDeadline.Seconds())),
			TTLSecondsAfterFinished: new(int32(ttlAfterFinished.Seconds())),
			Suspend:                 new(suspend),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					HostNetwork:   true,
					DNSPolicy:     corev1.DNSClusterFirstWithHostNet,
					NodeName:      node,
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:            "hvperf",
							Image:           imageName,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/bin/bash", "-c", fmt.Sprintf("sleep %v", jobActiveDeadline.Seconds())},
							SecurityContext: &corev1.SecurityContext{
								RunAsUser:  new(int64(0)),
								Privileged: new(true),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "rancher",
									MountPath: "/host/rancher",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "rancher",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/var/lib/rancher",
									Type: new(corev1.HostPathDirectory),
								},
							},
						},
					},
				},
			},
		},
	}

	// if any overrides are provided, apply them to the job spec
	for _, mod := range overrrides {
		if err := mod(job); err != nil {
			return nil, nil, err
		}
	}

	created, err := c.K8sClientSet.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return nil, nil, err
	}
	created.SetGroupVersionKind(batchv1.SchemeGroupVersion.WithKind("Job"))

	// wait till job pods are ready
	labelSelector := batchv1.JobNameLabel
	listWatch := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, options metav1.ListOptions) (runtime.Object, error) {
			options.LabelSelector = fmt.Sprintf("%s=%s", labelSelector, created.GetName())
			return c.K8sClientSet.CoreV1().Pods(created.GetNamespace()).List(ctx, options)
		},
		WatchFuncWithContext: func(ctx context.Context, options metav1.ListOptions) (watchapi.Interface, error) {
			options.LabelSelector = fmt.Sprintf("%s=%s", labelSelector, created.GetName())
			return c.K8sClientSet.CoreV1().Pods(created.GetNamespace()).Watch(ctx, options)
		},
	}
	ctxWithTimeout, cancel := watch.ContextWithOptionalTimeout(ctx, readyTimeout)
	defer cancel()
	event, err := watch.UntilWithSync(ctxWithTimeout, listWatch, &corev1.Pod{}, nil, func(event watchapi.Event) (bool, error) {
		pod, ok := event.Object.(*corev1.Pod)
		if !ok {
			return false, fmt.Errorf("unexpected object type: %T", event.Object)
		}

		// check if pod status has a Ready condition set to True
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return nil, nil, err
	}

	jobPod := event.Object.(*corev1.Pod)
	jobPod.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Pod"))
	return created, jobPod, nil
}
