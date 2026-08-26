package etcd

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/harvester/hvperf/pkg/k8s"
	pkgsuites "github.com/harvester/hvperf/pkg/suites"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
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

	// ensure the job is created and ready
	job, pod, err := k8s.EnsureJobReady(ctx, s.Clients,
		s.Name(),
		o.TestRunID,
		o.JobPodNamespace,
		o.JobPodNode,
		o.JobPodImageName+":"+o.JobPodImageTag,
		o.JobActiveDeadline,
		o.JobPodReadyTimeout,
		o.JobPodTTLAfterFinished,
		o.JobSuspend)
	if err != nil {
		return pkgsuites.SuiteResult{}, err
	}
	klog.V(3).Infof("pod:'%s' is now ready, phase:'%s'\n", pod.GetName(), pod.Status.Phase)

	// copy the etcdctl and benchmark binaries to the job pod. the job pod has a
	// host mount to /var/lib/rancher, where the etcd tls certs are stored.
	klog.V(3).Infof("copying '%s' to pod '%s'\n",
		fmt.Sprintf("%s,%s", o.EtcdctlLocalPath, o.EtcdBenchmarkLocalPath),
		pod.GetName())
	if err := k8s.CopyToJobPod(ctx,
		s.Clients,
		pod,
		o.EtcdRemoteCopyTargetDir,
		o.EtcdctlLocalPath,
		o.EtcdBenchmarkLocalPath); err != nil {
		return pkgsuites.SuiteResult{}, err
	}

	// issue exec command to run the etcdctl tool in the job pod
	klog.V(3).Infof("running etcdctl healthcheck in pod '%s'\n", pod.GetName())
	var caseResults []*pkgsuites.CaseResult
	dateTimeStart := time.Now()
	healthOut, err := s.execHealthcheck(ctx, pod, o, s.args(o)...)
	caseResults = append(caseResults, &pkgsuites.CaseResult{
		CaseName:      "etcd healthcheck",
		DateTimeStart: dateTimeStart,
		DateTimeEnd:   time.Now(),
		Objects:       []runtime.Object{job, pod},
		Success:       err == nil,
		Out:           string(healthOut),
		Err:           err,
	})

	// issue exec command to run the benchmark tool in the job pod
	klog.V(3).Infof("running etcd benchmark in pod '%s'\n", pod.GetName())
	dateTimeStart = time.Now()
	benchOut, err := s.execBenchmark(ctx, pod, o, s.args(o)...)
	caseResults = append(caseResults, &pkgsuites.CaseResult{
		CaseName:      "etcd benchmark",
		DateTimeStart: dateTimeStart,
		DateTimeEnd:   time.Now(),
		Objects:       []runtime.Object{job, pod},
		Success:       err == nil,
		Out:           string(benchOut),
		Err:           err,
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
) ([]byte, error) {
	outArgs := []string{
		"-w", opts.EtcdctlOutputFormat,
	}
	cmds := [][]string{
		{"etcdctl", "endpoint", "status"},
		{"etcdctl", "endpoint", "health"},
		{"etcdctl", "member", "list"},
	}

	cmdWithArgs := [][]string{}
	for _, cmd := range cmds {
		args = append(args, outArgs...)
		cmd = append(cmd, args...)
		cmdWithArgs = append(cmdWithArgs, cmd)
	}
	r, err := k8s.ExecPod(ctx, s.Clients, pod, cmdWithArgs)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

func (s *BenchmarkSuite) execBenchmark(
	ctx context.Context,
	pod *corev1.Pod,
	opts *BenchmarkOptions,
	args ...string,
) ([]byte, error) {
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

	cmdWithArgs := [][]string{}
	for _, cmd := range cmds {
		cmd = append(cmd, args...)
		cmdWithArgs = append(cmdWithArgs, cmd)
	}
	r, err := k8s.ExecPod(ctx, s.Clients, pod, cmdWithArgs)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
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
		EtcdctlOutputFormat:     "simple",
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
