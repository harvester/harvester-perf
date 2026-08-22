package suites

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	pkgsuites "github.com/harvester/hperf/pkg/suites"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	watchapi "k8s.io/apimachinery/pkg/watch"
	"k8s.io/kubectl/pkg/scheme"

	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/tools/watch"
)

func init() {
	pkgsuites.Register(NewEtcdBenchmarkSuite())
}

var _ pkgsuites.Suite = &EtcdBenchmarkSuite{}

// EtcdBenchmarkSuite implements test suite to assess etcd performance.
type EtcdBenchmarkSuite struct {
	pkgsuites.SuiteMarshaler
	*pkgsuites.Clients
}

// NewEtcdBenchmarkSuite creates a new instance of EtcdBenchmarkSuite with the
// provided options.
func NewEtcdBenchmarkSuite() *EtcdBenchmarkSuite {
	s := &EtcdBenchmarkSuite{}
	s.Marshal = s
	return s
}

func (s *EtcdBenchmarkSuite) Name() string {
	return "etcd-benchmark"
}

func (s *EtcdBenchmarkSuite) Description() string {
	return "run the etcd-benchmark tool to against the cluster's etcd"
}

func (s *EtcdBenchmarkSuite) IsReadWrite() bool {
	return true
}

func (s *EtcdBenchmarkSuite) RunE(ctx context.Context, opts pkgsuites.Options) (pkgsuites.SuiteResult, error) {
	o := EtcdBenchmarkSuiteOptionsDefaults()
	// custom, err := FromOptions(opts["etcd-benchmark"])
	// if err != nil {
	// 	return pkgsuites.SuiteResult{}, err
	// }
	// // TODO: merge custom options into defaults
	// fmt.Fprintf(os.Stderr, "[info] test run ID: %s\n", custom.TestRunID)

	// ensure the job is created and ready
	job, pod, err := s.ensureJobReady(ctx, o)
	if err != nil {
		return pkgsuites.SuiteResult{}, err
	}
	fmt.Fprintf(os.Stderr, "[info] job:'%s',pod:'%s',msg:'is now ready',phase:'%s')\n", job.GetName(), pod.GetName(), pod.Status.Phase)

	// copy the etcdctl and benchmark binaries to the job pod. the job pod has a
	// host mount to /var/lib/rancher, where the etcd tls certs are stored.
	if err := s.copyToolsToJobPod(ctx, pod, o); err != nil {
		return pkgsuites.SuiteResult{}, err
	}

	// issue exec command to run the etcdclt and benchmark tool in the job pod
	out, outerr, err := s.execBenchmark(ctx, pod, o)
	if err != nil {
		return pkgsuites.SuiteResult{}, err
	}

	return pkgsuites.SuiteResult{
		TestSuiteName: s.Name(),
		TestRunID:     o.TestRunID,
		Out:           out,
		Err:           outerr,
	}, nil
}

func (s *EtcdBenchmarkSuite) ensureJobReady(ctx context.Context, opts *EtcdBenchmarkSuiteOptions) (*batchv1.Job, *corev1.Pod, error) {
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
							Command:         []string{"/bin/bash", "-c", "sleep infinity"},
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
	event, err := watch.UntilWithSync(ctx, listWatch, &corev1.Pod{}, nil, func(event watchapi.Event) (bool, error) {
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

func (s *EtcdBenchmarkSuite) copyToolsToJobPod(ctx context.Context, pod *corev1.Pod, opts *EtcdBenchmarkSuiteOptions) error {
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

		err = exec.StreamWithContext(context.Background(), remotecommand.StreamOptions{
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

func (s *EtcdBenchmarkSuite) execBenchmark(
	ctx context.Context,
	pod *corev1.Pod,
	opts *EtcdBenchmarkSuiteOptions,
) (string, string, error) {
	tlsArgs := []string{
		"--cacert", fmt.Sprintf("%s/server-ca.crt", opts.EtcdRemoteTLSCertDir),
		"--cert", fmt.Sprintf("%s/server-client.crt", opts.EtcdRemoteTLSCertDir),
		"--key", fmt.Sprintf("%s/server-client.key", opts.EtcdRemoteTLSCertDir),
	}
	outArgs := []string{
		"-w", "json",
	}
	endpointsArgs := []string{
		"--endpoints", opts.EtcdEndpoints,
	}
	cmds := [][]string{
		{"etcdctl", "endpoint", "status"},
		{"etcdctl", "endpoint", "health"},
		{"etcdctl", "member", "list"},
	}

	var (
		errs   error
		bufOut = &bytes.Buffer{}
		bufErr = &bytes.Buffer{}
	)
	for _, cmd := range cmds {
		cmd = append(cmd, endpointsArgs...)
		cmd = append(cmd, tlsArgs...)
		cmd = append(cmd, outArgs...)
		req := s.K8sClientSet.CoreV1().RESTClient().
			Post().
			Resource("pods").
			Name(pod.GetName()).
			Namespace(pod.GetNamespace()).
			SubResource("exec").
			VersionedParams(&corev1.PodExecOptions{
				Container: opts.JobPodContainerName,
				Command:   cmd,
				Stdin:     false,
				Stdout:    true,
				Stderr:    true,
				TTY:       false,
			}, scheme.ParameterCodec)
		fmt.Fprintf(os.Stderr, "[info] remote exec to pod '%s'\n", pod.GetName())
		fmt.Fprintf(os.Stderr, "[debug] exec cmd: %s\n", strings.Join(cmd, " "))

		// setup spdy executor and exec the command in the pod
		exec, err := remotecommand.NewSPDYExecutor(s.RestConfig, "POST", req.URL())
		if err != nil {
			return "", "", fmt.Errorf("failed to init SPDY executor: %w", err)
		}

		var (
			bout = &bytes.Buffer{}
			berr = &bytes.Buffer{}
		)
		if err := exec.StreamWithContext(context.Background(), remotecommand.StreamOptions{
			Stdout: bout,
			Stderr: berr,
		}); err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to exec command '%s': %w", strings.Join(cmd, " "), err))
			continue
		}

		// don't return on write-to-buffer error here, instead collect the stdout and
		// stderr buffers for  all commands
		if _, err := bufOut.Write(bout.Bytes()); err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to write stdout buffer: %w", err))
		}
		if _, err := bufErr.Write(berr.Bytes()); err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to write stderr buffer: %w", err))
		}
	}
	return bufOut.String(), bufErr.String(), errs
}

func (s *EtcdBenchmarkSuite) SetClients(clients *pkgsuites.Clients) {
	s.Clients = clients
}

type EtcdBenchmarkSuiteOptions struct {
	TestRunID string

	EtcdBenchmarkLocalPath  string
	EtcdctlLocalPath        string
	EtcdEndpoints           string
	EtcdMetricsPort         int
	EtcdRemoteTLSCertDir    string
	EtcdRemoteCopyTargetDir string

	JobActiveDeadline      time.Duration
	JobPodContainerName    string
	JobPodImageName        string
	JobPodImageTag         string
	JobPodNamespace        string
	JobPodNode             string
	JobPodTTLAfterFinished time.Duration
	JobSuspend             bool

	LoadSize   EtcdBenchmarkLoadSize
	ConnSize   string
	ClientSize string
	ValSize    string
}

func EtcdBenchmarkSuiteOptionsDefaults() *EtcdBenchmarkSuiteOptions {
	return &EtcdBenchmarkSuiteOptions{
		TestRunID: time.Now().Format("20060102150405"),

		EtcdBenchmarkLocalPath:  "/usr/local/bin/benchmark",
		EtcdctlLocalPath:        "/usr/local/bin/etcdctl",
		EtcdEndpoints:           "https://127.0.0.1:2379",
		EtcdRemoteCopyTargetDir: "/usr/local/bin/",
		EtcdRemoteTLSCertDir:    "/host/rancher/rke2/server/tls/etcd",

		JobActiveDeadline:      3600 * time.Second,
		JobPodContainerName:    "benchmark",
		JobPodImageName:        "registry.suse.com/bci/bci-base",
		JobPodImageTag:         "latest",
		JobPodNamespace:        "default",
		JobPodTTLAfterFinished: 3600 * time.Second,
	}
}

type EtcdBenchmarkLoadSize uint64

const (
	EtcdBenchmarkLoadSizeSmall  EtcdBenchmarkLoadSize = 10000
	EtcdBenchmarkLoadSizeMedium EtcdBenchmarkLoadSize = 50000
)
