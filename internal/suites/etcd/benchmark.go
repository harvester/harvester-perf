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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/scheme"
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

	// issue exec command to run the etcdctl tool in the job pod
	var caseResults []*pkgsuites.CaseResult
	healthOut, healthOutErr, err := s.execHealthcheck(ctx, pod, o, s.args(o)...)
	caseResults = append(caseResults, &pkgsuites.CaseResult{
		Description: "etcd healthcheck",
		ObjMeta:     []metav1.Object{job, pod},
		Success:     err != nil,
		Out:         healthOut,
		OutErr:      healthOutErr,
		Err:         err,
	})

	// issue exec command to run the benchmark tool in the job pod
	benchOut, benchOutErr, err := s.execBenchmark(ctx, pod, o, s.args(o)...)
	caseResults = append(caseResults, &pkgsuites.CaseResult{
		Description: "etcd benchmark",
		ObjMeta:     []metav1.Object{job, pod},
		Success:     err != nil,
		Out:         benchOut,
		OutErr:      benchOutErr,
		Err:         err,
	})

	return pkgsuites.SuiteResult{
		Name:    s.Name(),
		RunID:   o.TestRunID,
		Results: caseResults,
	}, nil
}

func (s *BenchmarkSuite) args(opts *BenchmarkOptions) []string {
	endpointsArgs := []string{
		"--endpoints", opts.EtcdEndpoints,
	}
	tlsArgs := []string{
		"--cacert", fmt.Sprintf("%s/server-ca.crt", opts.EtcdRemoteTLSCertDir),
		"--cert", fmt.Sprintf("%s/server-client.crt", opts.EtcdRemoteTLSCertDir),
		"--key", fmt.Sprintf("%s/server-client.key", opts.EtcdRemoteTLSCertDir),
	}
	return append(endpointsArgs, tlsArgs...)
}

func (s *BenchmarkSuite) execHealthcheck(
	ctx context.Context,
	pod *corev1.Pod,
	opts *BenchmarkOptions,
	args ...string,
) (string, string, error) {
	outArgs := []string{
		"-w", opts.EtcdctlOutputFormat,
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
		cmd = append(cmd, args...)
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

func (s *BenchmarkSuite) execBenchmark(
	ctx context.Context,
	pod *corev1.Pod,
	opts *BenchmarkOptions,
	args ...string,
) (string, string, error) {
	cmds := [][]string{
		{
			"benchmark",
			"--conns", fmt.Sprintf("%v", opts.GRPCConnCount),
			"--clients", fmt.Sprintf("%v", opts.GRPCClientCount),
			"put",
			"--key-size", fmt.Sprintf("%v", "8"),
			"--sequential-keys",
			"--total", fmt.Sprintf("%v", opts.PutLoadSize),
			"--val-size", fmt.Sprintf("%v", opts.PutValSize),
		},
		{
			"benchmark",
			"--conns", fmt.Sprintf("%v", opts.GRPCConnCount),
			"--clients", fmt.Sprintf("%v", opts.GRPCClientCount),
			"range",
			"hvperf-probe",
			"--consistency", opts.RangeConsistency,
			"--total", fmt.Sprintf("%v", opts.PutLoadSize),
		},
	}

	var (
		errs   error
		bufOut = &bytes.Buffer{}
		bufErr = &bytes.Buffer{}
	)
	for _, cmd := range cmds {
		cmd = append(cmd, args...)
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

	PutLoadSize      PutLoadSize
	PutValSize       PutValSize
	RangeConsistency string
	GRPCConnCount    GRPCConnCount
	GRPCClientCount  GRPCClientCount
}

func EtcdBenchmarkSuiteOptionsDefaults() *BenchmarkOptions {
	return &BenchmarkOptions{
		TestRunID: time.Now().Format("20060102150405"),

		EtcdBenchmarkLocalPath:  "/usr/local/bin/benchmark",
		EtcdctlLocalPath:        "/usr/local/bin/etcdctl",
		EtcdctlOutputFormat:     "json",
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

		PutLoadSize:      DefaultLoadSize,
		PutValSize:       DefaultPutValSize,
		RangeConsistency: "l",
		GRPCClientCount:  DefaultClientCount,
		GRPCConnCount:    DefaultConnCount,
	}
}

type (
	// PutLoadSize represents the total number of put requests
	PutLoadSize uint64

	// PutKeySize represents the size of the key in bytes for each put request
	PutKeySize uint64

	// PutValSize represents the size of the value in bytes for each put request
	PutValSize uint64

	// GRPCClientCount represents the number of grpc clients
	GRPCClientCount uint64

	// GRPCConnCount represents the number of grpc connections
	GRPCConnCount uint64
)

const (
	DefaultLoadSize    PutLoadSize     = 10000
	DefaultKeySize     PutKeySize      = 8
	DefaultPutValSize  PutValSize      = 256
	DefaultClientCount GRPCClientCount = 1
	DefaultConnCount   GRPCConnCount   = 1
)
