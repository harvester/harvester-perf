package etcd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	pkgsuites "github.com/harvester/hvperf/pkg/suites"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/kubectl/pkg/scheme"

	"k8s.io/client-go/tools/remotecommand"
)

func init() {
	pkgsuites.Register(NewBenchmarkSuite())
}

var _ pkgsuites.Suite = &BenchmarkSuite{}

// BenchmarkSuite implements test suite to assess etcd performance.
type BenchmarkSuite struct {
	pkgsuites.SuiteMarshaler
	*pkgsuites.Clients
}

// NewBenchmarkSuite creates a new instance of EtcdBenchmarkSuite with the
// provided options.
func NewBenchmarkSuite() *BenchmarkSuite {
	s := &BenchmarkSuite{}
	s.Marshal = s
	return s
}

func (s *BenchmarkSuite) Name() string {
	return "etcd-benchmark"
}

func (s *BenchmarkSuite) Description() string {
	return "run the etcd-benchmark tool to against the cluster's etcd"
}

func (s *BenchmarkSuite) IsReadWrite() bool {
	return true
}

func (s *BenchmarkSuite) RunE(ctx context.Context, opts pkgsuites.Options) (pkgsuites.SuiteResult, error) {
	o := EtcdBenchmarkSuiteOptionsDefaults()
	// custom, err := FromOptions(opts["etcd-benchmark"])
	// if err != nil {
	// 	return pkgsuites.SuiteResult{}, err
	// }
	// // TODO: merge custom options into defaults
	// fmt.Fprintf(os.Stderr, "[info] test run ID: %s\n", custom.TestRunID)

	// ensure the job is created and ready
	job, pod, err := s.ensureJobReady(ctx, o, o.JobPodReadyTimeout)
	if err != nil {
		return pkgsuites.SuiteResult{}, err
	}
	fmt.Fprintf(os.Stderr, "[info] job:'%s',pod:'%s',msg:'is now ready',phase:'%s'\n", job.GetName(), pod.GetName(), pod.Status.Phase)

	// copy the etcdctl and benchmark binaries to the job pod. the job pod has a
	// host mount to /var/lib/rancher, where the etcd tls certs are stored.
	if err := s.copyToolsToJobPod(ctx, pod, o); err != nil {
		return pkgsuites.SuiteResult{}, err
	}

	// issue exec command to run the etcdclt and benchmark tool in the job pod
	out, outerr, err := s.execHealthcheck(ctx, pod, o)
	if err != nil {
		return pkgsuites.SuiteResult{}, err
	}

	return pkgsuites.SuiteResult{
		TestSuiteName: s.Name(),
		TestRunID:     o.TestRunID,
		IsSuccess:     outerr == "",
		Out:           out,
		Err:           outerr,
	}, nil
}

func (s *BenchmarkSuite) execHealthcheck(
	ctx context.Context,
	pod *corev1.Pod,
	opts *BenchmarkOptions,
) (string, string, error) {
	tlsArgs := []string{
		"--cacert", fmt.Sprintf("%s/server-ca.crt", opts.EtcdRemoteTLSCertDir),
		"--cert", fmt.Sprintf("%s/server-client.crt", opts.EtcdRemoteTLSCertDir),
		"--key", fmt.Sprintf("%s/server-client.key", opts.EtcdRemoteTLSCertDir),
	}
	outArgs := []string{
		"-w", opts.EtcdctlOutputFormat,
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
		if err := exec.StreamWithContext(ctx, remotecommand.StreamOptions{
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

func (s *BenchmarkSuite) SetClients(clients *pkgsuites.Clients) {
	s.Clients = clients
}

type BenchmarkOptions struct {
	TestRunID string

	EtcdBenchmarkLocalPath  string
	EtcdctlLocalPath        string
	EtcdctlOutputFormat     string
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
	JobPodReadyTimeout     time.Duration
	JobSuspend             bool

	LoadSize   EtcdBenchmarkLoadSize
	ConnSize   string
	ClientSize string
	ValSize    string
}

func EtcdBenchmarkSuiteOptionsDefaults() *BenchmarkOptions {
	return &BenchmarkOptions{
		TestRunID: time.Now().Format("20060102150405"),

		EtcdBenchmarkLocalPath:  "/usr/local/bin/benchmark",
		EtcdctlLocalPath:        "/usr/local/bin/etcdctl",
		EtcdctlOutputFormat:     "table",
		EtcdEndpoints:           "https://127.0.0.1:2379",
		EtcdRemoteCopyTargetDir: "/usr/local/bin/",
		EtcdRemoteTLSCertDir:    "/host/rancher/rke2/server/tls/etcd",

		JobActiveDeadline:      3600 * time.Second,
		JobPodContainerName:    "benchmark",
		JobPodImageName:        "registry.suse.com/bci/bci-base",
		JobPodImageTag:         "latest",
		JobPodNamespace:        "default",
		JobPodReadyTimeout:     300 * time.Second,
		JobPodTTLAfterFinished: 3600 * time.Second,
	}
}

type EtcdBenchmarkLoadSize uint64

const (
	EtcdBenchmarkLoadSizeSmall  EtcdBenchmarkLoadSize = 10000
	EtcdBenchmarkLoadSizeMedium EtcdBenchmarkLoadSize = 50000
)
