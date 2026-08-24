package etcd

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	watchapi "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/tools/watch"
	"k8s.io/kubectl/pkg/scheme"
)

func (s *BenchmarkSuite) ensureJobReady(
	ctx context.Context,
	opts *BenchmarkOptions,
	podReadyTimeout time.Duration,
) (*batchv1.Job, *corev1.Pod, error) {
	jobName := "etcd-benchmark-" + opts.TestRunID
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: opts.JobPodNamespace,
		},
		Spec: batchv1.JobSpec{
			ActiveDeadlineSeconds:   new(int64(opts.JobActiveDeadline.Seconds())),
			TTLSecondsAfterFinished: new(int32(opts.JobPodTTLAfterFinished.Seconds())),
			Suspend:                 new(opts.JobSuspend),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					HostNetwork:   true,
					NodeName:      opts.JobPodNode,
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:            opts.JobPodContainerName,
							Image:           opts.JobPodImageName,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/bin/bash", "-c", fmt.Sprintf("sleep %s", opts.JobActiveDeadline.Seconds())},
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
	created, err := s.K8sClientSet.BatchV1().Jobs(opts.JobPodNamespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return nil, nil, err
	}

	// wait till job pods are ready
	labelSelector := batchv1.JobNameLabel
	listWatch := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, options metav1.ListOptions) (runtime.Object, error) {
			options.LabelSelector = fmt.Sprintf("%s=%s", labelSelector, created.GetName())
			return s.K8sClientSet.CoreV1().Pods(created.GetNamespace()).List(ctx, options)
		},
		WatchFuncWithContext: func(ctx context.Context, options metav1.ListOptions) (watchapi.Interface, error) {
			options.LabelSelector = fmt.Sprintf("%s=%s", labelSelector, created.GetName())
			return s.K8sClientSet.CoreV1().Pods(created.GetNamespace()).Watch(ctx, options)
		},
	}
	ctxWithTimeout, cancel := watch.ContextWithOptionalTimeout(ctx, podReadyTimeout)
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
	return job, jobPod, nil
}

func (s *BenchmarkSuite) copyToolsToJobPod(ctx context.Context, pod *corev1.Pod, opts *BenchmarkOptions) error {
	var deferFuncs []func() error
	defer func() {
		for _, f := range deferFuncs {
			//nolint:errcheck
			f()
		}
	}()

	for _, path := range []string{opts.EtcdctlLocalPath, opts.EtcdBenchmarkLocalPath} {
		fileInfo, err := os.Stat(path)
		if err != nil {
			return err
		}
		bin, err := os.Open(path)
		if err != nil {
			return err
		}
		deferFuncs = append(deferFuncs, bin.Close)

		// archive the bin as a tar before sending to the cluster
		var buf bytes.Buffer
		tarWriter := tar.NewWriter(&buf)
		if err := tarWriter.WriteHeader(&tar.Header{
			Name: fileInfo.Name(),
			Mode: 0o755,
			Size: fileInfo.Size(),
		}); err != nil {
			return fmt.Errorf("failed to write tar header: %w", err)
		}
		if _, err := io.Copy(tarWriter, bin); err != nil {
			return fmt.Errorf("failed to copy file to tar: %w", err)
		}
		if err := tarWriter.Close(); err != nil {
			return fmt.Errorf("failed to close tar writer: %w", err)
		}

		// exec command: tar -xf - -C <remoteDir>
		cmd := []string{"tar", "-xf", "-", "-C", opts.EtcdRemoteCopyTargetDir}
		req := s.K8sClientSet.CoreV1().RESTClient().
			Post().
			Resource("pods").
			Name(pod.GetName()).
			Namespace(pod.GetNamespace()).
			SubResource("exec").
			VersionedParams(&corev1.PodExecOptions{
				Container: opts.JobPodContainerName,
				Command:   cmd,
				Stdin:     true,
				Stdout:    true,
				Stderr:    true,
				TTY:       false,
			}, scheme.ParameterCodec)

		// setup spdy executor and stream the tar to the pod's stdin
		fmt.Fprintf(os.Stderr, "[info] copying '%s' to pod '%s'\n", path, pod.GetName())
		exec, err := remotecommand.NewSPDYExecutor(s.RestConfig, "POST", req.URL())
		if err != nil {
			return fmt.Errorf("failed to init SPDY executor: %w", err)
		}

		err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdin:  &buf,
			Stdout: os.Stdout,
			Stderr: os.Stderr,
		})
		if err != nil {
			return fmt.Errorf("failed to stream binary: %w", err)
		}
	}
	return nil
}
