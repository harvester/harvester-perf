package etcd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/harvester/hvperf/internal/suites/options"
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

func (s *BenchmarkSuite) RunE(
	ctx context.Context,
	runID string,
	namespace string,
	opts pkgsuites.Options,
) (pkgsuites.SuiteResult, error) {
	o, err := BenchmarkOptionsDefaults()
	if err != nil {
		return pkgsuites.SuiteResult{}, err
	}

	// TODO: merge custom options
	// custom, err := FromOptions(opts)
	// if err != nil {
	// 	return pkgsuites.SuiteResult{}, err
	// }

	nsReadyTimeout := 60 * time.Second
	if _, err := k8s.EnsureNamespace(ctx, s.Clients, namespace, nsReadyTimeout); err != nil {
		return pkgsuites.SuiteResult{}, err
	}
	klog.V(3).Infof("namespace:'%s' is now ready\n", namespace)

	// ensure the job is created and ready
	job, pod, err := k8s.EnsureJobReady(ctx, s.Clients,
		s.Name(),
		runID,
		namespace,
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

	checkPerfOut, err := s.execCheckPerf(ctx, pod, o, s.args(o)...)
	caseResults = append(caseResults, &pkgsuites.CaseResult{
		CaseName:      "etcd check perf",
		DateTimeStart: dateTimeStart,
		DateTimeEnd:   time.Now(),
		Objects:       []runtime.Object{job, pod},
		Success:       err == nil,
		Out:           string(checkPerfOut),
		Err:           err,
	})

	// issue exec commands to run the benchmark tool in the job pod
	klog.V(3).Infof("running etcd benchmark (serial suite) in pod '%s'\n", pod.GetName())
	dateTimeStart = time.Now()
	benchSerialOut, err := s.execBenchmark(ctx, pod, o, s.args(o)...)
	caseResults = append(caseResults, &pkgsuites.CaseResult{
		CaseName:      "etcd benchmark",
		DateTimeStart: dateTimeStart,
		DateTimeEnd:   time.Now(),
		Objects:       []runtime.Object{job, pod},
		Success:       err == nil,
		Out:           string(benchSerialOut),
		Err:           err,
	})

	klog.V(3).Infof("running etcd benchmark (concurrent suite) in pod '%s'\n", pod.GetName())
	dateTimeStart = time.Now()
	o.PutLoadSize = DefaultConcurrentLoadSize
	o.GRPCClientCount = DefaultConcurrentClientCount
	o.GRPCConnCount = DefaultConcurrentConnCount
	benchConcurrentOut, err := s.execBenchmark(ctx, pod, o, s.args(o)...)
	caseResults = append(caseResults, &pkgsuites.CaseResult{
		CaseName:      "etcd benchmark",
		DateTimeStart: dateTimeStart,
		DateTimeEnd:   time.Now(),
		Objects:       []runtime.Object{job, pod},
		Success:       err == nil,
		Out:           string(benchConcurrentOut),
		Err:           err,
	})

	return pkgsuites.SuiteResult{
		Name:    s.Name(),
		RunID:   runID,
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
	args = append(args, outArgs...)
	for _, cmd := range cmds {
		cmd = append(cmd, args...)
		cmdWithArgs = append(cmdWithArgs, cmd)
	}
	r, err := k8s.ExecPod(ctx, s.Clients, pod, cmdWithArgs)
	// don't discard any partial outputs
	if r != nil {
		b, readErr := io.ReadAll(r)
		return b, errors.Join(err, readErr)
	}
	return nil, err
}

func (s *BenchmarkSuite) execCheckPerf(
	ctx context.Context,
	pod *corev1.Pod,
	opts *BenchmarkOptions,
	args ...string,
) ([]byte, error) {
	outArgs := []string{
		"-w", opts.EtcdctlOutputFormat,
		"--load", opts.CheckPerfLoadSize,
	}
	cmds := [][]string{
		{"etcdctl", "check", "perf"},
	}

	cmdWithArgs := [][]string{}
	args = append(args, outArgs...)
	for _, cmd := range cmds {
		cmd = append(cmd, args...)
		cmdWithArgs = append(cmdWithArgs, cmd)
	}
	r, err := k8s.ExecPod(ctx, s.Clients, pod, cmdWithArgs)
	// don't discard any partial outputs
	if r != nil {
		b, readErr := io.ReadAll(r)
		return b, errors.Join(err, readErr)
	}
	return nil, err
}

func (s *BenchmarkSuite) execBenchmark(
	ctx context.Context,
	pod *corev1.Pod,
	opts *BenchmarkOptions,
	args ...string,
) ([]byte, error) {
	cmds := [][]string{
		{
			// write serial
			"benchmark",
			"--conns", fmt.Sprintf("%v", opts.GRPCConnCount),
			"--clients", fmt.Sprintf("%v", opts.GRPCClientCount),
			"put",
			"--key-size", fmt.Sprintf("%v", opts.PutKeySize),
			"--sequential-keys",
			"--total", fmt.Sprintf("%v", opts.PutLoadSize),
			"--val-size", fmt.Sprintf("%v", opts.PutValSize),
		},
		{
			// read linearizable
			"benchmark",
			"--conns", fmt.Sprintf("%v", opts.GRPCConnCount),
			"--clients", fmt.Sprintf("%v", opts.GRPCClientCount),
			"range",
			"hvperf-probe",
			"--consistency", DefaultRangeConsistencyLinearizable,
			"--total", fmt.Sprintf("%v", opts.PutLoadSize),
		},
		{
			// read serializable
			"benchmark",
			"--conns", fmt.Sprintf("%v", opts.GRPCConnCount),
			"--clients", fmt.Sprintf("%v", opts.GRPCClientCount),
			"range",
			"hvperf-probe",
			"--consistency", DefaultRangeConsistencySerializable,
			"--total", fmt.Sprintf("%v", opts.PutLoadSize),
		},
	}

	cmdWithArgs := [][]string{}
	for _, cmd := range cmds {
		cmd = append(cmd, args...)
		cmdWithArgs = append(cmdWithArgs, cmd)
	}
	r, err := k8s.ExecPod(ctx, s.Clients, pod, cmdWithArgs)
	// don't discard any partial outputs
	if r != nil {
		b, readErr := io.ReadAll(r)
		return b, errors.Join(err, readErr)
	}
	return nil, err
}

func (s *BenchmarkSuite) SetClients(clients *pkgsuites.Clients) {
	s.Clients = clients
}

type BenchmarkOptions struct {
	EtcdBenchmarkLocalPath  string
	EtcdctlLocalPath        string
	EtcdctlOutputFormat     string
	EtcdEndpoints           string
	EtcdMetricsPort         int
	EtcdRemoteTLSCertDir    string
	EtcdRemoteCopyTargetDir string

	JobActiveDeadline      time.Duration
	JobKeepAlive           bool
	JobPodContainerName    string
	JobPodImageName        string
	JobPodImageTag         string
	JobPodNode             string
	JobPodTTLAfterFinished time.Duration
	JobPodReadyTimeout     time.Duration
	JobSuspend             bool

	CheckPerfLoadSize string
	PutLoadSize       uint64
	PutKeySize        uint64
	PutValSize        uint64
	GRPCConnCount     uint64
	GRPCClientCount   uint64
}

// BenchmarkOptionsDefaults returns the default options for the etcd benchmark
// suite. It merges suite-specific options with the default global options for
// the all test suites.
func BenchmarkOptionsDefaults() (*BenchmarkOptions, error) {
	sysOpts, err := options.FromOptions[*BenchmarkOptions](pkgsuites.DefaultGlobalOptions())
	if err != nil {
		return nil, err
	}

	// TODO find a better way to merge the two structs
	benchmarkOptions := &BenchmarkOptions{
		EtcdBenchmarkLocalPath:  "/usr/local/bin/benchmark",
		EtcdctlLocalPath:        "/usr/local/bin/etcdctl",
		EtcdctlOutputFormat:     "simple",
		EtcdEndpoints:           "https://127.0.0.1:2379",
		EtcdRemoteCopyTargetDir: "/usr/local/bin/",
		EtcdRemoteTLSCertDir:    "/host/rancher/rke2/server/tls/etcd",

		JobActiveDeadline:      sysOpts.JobActiveDeadline,
		JobKeepAlive:           sysOpts.JobKeepAlive,
		JobPodContainerName:    sysOpts.JobPodContainerName,
		JobPodImageName:        sysOpts.JobPodImageName,
		JobPodImageTag:         sysOpts.JobPodImageTag,
		JobPodNode:             sysOpts.JobPodNode,
		JobPodTTLAfterFinished: sysOpts.JobPodTTLAfterFinished,
		JobPodReadyTimeout:     sysOpts.JobPodReadyTimeout,
		JobSuspend:             sysOpts.JobSuspend,

		CheckPerfLoadSize: DefaultCheckPerfLoadSize,
		PutLoadSize:       DefaultLoadSize,
		PutKeySize:        DefaultKeySize,
		PutValSize:        DefaultPutValSize,
		GRPCClientCount:   DefaultClientCount,
		GRPCConnCount:     DefaultConnCount,
	}

	return benchmarkOptions, nil
}

const (
	DefaultConcurrentClientCount        = 500
	DefaultConcurrentConnCount          = 100
	DefaultConcurrentLoadSize           = 50000
	DefaultRangeConsistencySerializable = "s"

	DefaultCheckPerfLoadSize            = "s"
	DefaultClientCount                  = 1
	DefaultConnCount                    = 1
	DefaultLoadSize                     = 10000
	DefaultKeySize                      = 8
	DefaultPutValSize                   = 256
	DefaultRangeConsistencyLinearizable = "l"
)
