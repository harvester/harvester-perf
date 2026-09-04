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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	if err := k8s.CopyToJobPod(
		ctx,
		s.Clients,
		pod,
		o.EtcdRemoteCopyTargetDir,
		o.EtcdctlLocalPath,
		o.EtcdBenchmarkLocalPath,
		o.PromtoolLocalPath,
	); err != nil {
		return pkgsuites.SuiteResult{}, err
	}

	// issue exec command to run the etcdctl tool in the job pod
	klog.V(3).Infof("running etcdctl healthcheck in pod '%s'\n", pod.GetName())
	var caseResults []*pkgsuites.CaseResult
	dateTimeStart := time.Now()
	healthOut, cmds, err := s.execHealthcheck(ctx, pod, o, s.args(o)...)
	caseResults = append(caseResults, &pkgsuites.CaseResult{
		CaseName:      "etcd healthcheck",
		Cmds:          cmds,
		DateTimeStart: dateTimeStart,
		DateTimeEnd:   time.Now(),
		Objects:       []runtime.Object{job, pod},
		Success:       err == nil,
		Out:           string(healthOut),
		Err:           err,
	})

	checkPerfOut, cmds, err := s.execCheckPerf(ctx, pod, o, s.args(o)...)
	caseResults = append(caseResults, &pkgsuites.CaseResult{
		CaseName:      "etcd check perf",
		Cmds:          cmds,
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
	benchSerialOut, cmds, err := s.execBenchmark(ctx, pod, o, s.args(o)...)
	caseResults = append(caseResults, &pkgsuites.CaseResult{
		CaseName:      "etcd benchmark",
		Cmds:          cmds,
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
	benchConcurrentOut, cmds, err := s.execBenchmark(ctx, pod, o, s.args(o)...)
	caseResults = append(caseResults, &pkgsuites.CaseResult{
		CaseName:      "etcd benchmark",
		Cmds:          cmds,
		DateTimeStart: dateTimeStart,
		DateTimeEnd:   time.Now(),
		Objects:       []runtime.Object{job, pod},
		Success:       err == nil,
		Out:           string(benchConcurrentOut),
		Err:           err,
	})

	klog.V(3).Infof("running etcd monitoring (promql suite) in pod '%s'\n", pod.GetName())
	dateTimeStart = time.Now()
	promqlOut, cmds, skipped, err := s.monitoring(ctx, pod, o)
	caseResults = append(caseResults, &pkgsuites.CaseResult{
		CaseName:      "etcd monitoring (promql)",
		Cmds:          cmds,
		DateTimeStart: dateTimeStart,
		DateTimeEnd:   time.Now(),
		Objects:       []runtime.Object{job, pod},
		Skipped:       skipped,
		Success:       err == nil,
		Out:           string(promqlOut),
		Err:           err,
	})

	suiteParams, err := pkgsuites.ToSuiteParams(o)
	if err != nil {
		// just log the params conversion error, don't fail the suite. the only effect
		// is that the suite params won't be reported in the results.
		klog.V(3).ErrorS(err, "failed to convert suite options to suite params\n")
	}
	return pkgsuites.SuiteResult{
		Name:    s.Name(),
		Params:  suiteParams,
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
) ([]byte, [][]string, error) {
	outArgs := []string{
		"-w", opts.EtcdctlOutputFormat,
	}
	cmds := [][]string{
		{"etcdctl", "endpoint", "status"},
		{"etcdctl", "endpoint", "health"},
		{"etcdctl", "member", "list"},
	}

	runCmds := [][]string{}
	args = append(args, outArgs...)
	for _, cmd := range cmds {
		cmd = append(cmd, args...)
		runCmds = append(runCmds, cmd)
	}
	r, err := k8s.ExecPod(ctx, s.Clients, pod, runCmds)
	// don't discard any partial outputs
	if r != nil {
		b, readErr := io.ReadAll(r)
		return b, runCmds, errors.Join(err, readErr)
	}
	return nil, runCmds, err
}

func (s *BenchmarkSuite) execCheckPerf(
	ctx context.Context,
	pod *corev1.Pod,
	opts *BenchmarkOptions,
	args ...string,
) ([]byte, [][]string, error) {
	outArgs := []string{
		"-w", opts.EtcdctlOutputFormat,
		"--load", opts.CheckPerfLoadSize,
	}
	cmds := [][]string{
		{"etcdctl", "check", "perf"},
	}

	runCmds := [][]string{}
	args = append(args, outArgs...)
	for _, cmd := range cmds {
		cmd = append(cmd, args...)
		runCmds = append(runCmds, cmd)
	}
	r, err := k8s.ExecPod(ctx, s.Clients, pod, runCmds)
	// don't discard any partial outputs
	if r != nil {
		b, readErr := io.ReadAll(r)
		return b, runCmds, errors.Join(err, readErr)
	}
	return nil, runCmds, err
}

func (s *BenchmarkSuite) execBenchmark(
	ctx context.Context,
	pod *corev1.Pod,
	opts *BenchmarkOptions,
	args ...string,
) ([]byte, [][]string, error) {
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

	runCmds := [][]string{}
	for _, cmd := range cmds {
		cmd = append(cmd, args...)
		runCmds = append(runCmds, cmd)
	}
	r, err := k8s.ExecPod(ctx, s.Clients, pod, runCmds)
	// don't discard any partial outputs
	if r != nil {
		b, readErr := io.ReadAll(r)
		return b, runCmds, errors.Join(err, readErr)
	}
	return nil, runCmds, err
}

func (s *BenchmarkSuite) monitoring(
	ctx context.Context,
	pod *corev1.Pod,
	opts *BenchmarkOptions,
	args ...string,
) ([]byte, [][]string, bool, error) {
	// check if monitoring addon is enabled and ready. if not, skip the promql
	// execution.
	ready, err := k8s.MonitoringEnabled(ctx, s.Clients, opts.MonitoringNamespace, opts.MonitoringAddonName)
	if err != nil {
		return nil, nil, false, err
	}

	// skip if not ready
	if !ready {
		klog.V(3).Infof("monitoring addon '%s' is not enabled in namespace '%s', skipping promql execution\n", opts.MonitoringNamespace, opts.MonitoringAddonName)
		return nil, nil, true, nil
	}

	// etcd metrics are not exposed by default, so we need to ensure that the pod
	// monitor is created
	podMonitorOpts := &k8s.PodMonitorOption{
		Name:                 s.Name(),
		Namespace:            pod.GetNamespace(),
		MetricsPortName:      opts.EtcdMetricsPortName,
		MetricsPath:          opts.EtcdMetricsPath,
		EndpointScheme:       opts.EtcdMetricsScheme,
		TargetNamespace:      opts.EtcdNamespace,
		MonitoringServiceURL: opts.MonitoringServiceURL,
		LabelSelector: map[string]string{
			"component": "etcd",
			"tier":      "control-plane",
		},
		WaitTimeout:    opts.MonitoringWaitPodMonitorTimeout,
		ScrapeInterval: opts.MonitoringScrapeInterval,
	}
	podMonErr := k8s.EnsurePodMonitor(
		ctx,
		s.Clients,
		podMonitorOpts,
		pod,
	)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()

		if err := s.MonClientSet.MonitoringV1().PodMonitors(pod.GetNamespace()).Delete(ctx, s.Name(), metav1.DeleteOptions{}); err != nil {
			if !apierrors.IsNotFound(err) {
				klog.V(3).ErrorS(err, "failed to delete pod monitor", "name", s.Name(), "namespace", pod.GetNamespace())
			}
		}
	}()
	if podMonErr != nil {
		return nil, nil, false, podMonErr
	}

	out, cmds, err := s.execPromQL(ctx, pod, opts, args...)
	if err != nil {
		return out, cmds, false, err
	}

	return out, cmds, false, err
}

func (s *BenchmarkSuite) execPromQL(
	ctx context.Context,
	pod *corev1.Pod,
	opts *BenchmarkOptions,
	args ...string,
) ([]byte, [][]string, error) {
	cmds := [][]string{
		{
			// p99 WAL fsync
			"promtool",
			"query",
			"instant",
			"-o",
			opts.MonitoringOutputFormat,
			opts.MonitoringServiceURL,
			fmt.Sprintf(`histogram_quantile(
				0.99,
				sum by (le, pod) (
					rate(etcd_disk_wal_fsync_duration_seconds_bucket{namespace='%s'}[5m])
				)
			)`, opts.EtcdNamespace),
		},
		{
			// p99 backend commit
			"promtool",
			"query",
			"instant",
			"-o",
			opts.MonitoringOutputFormat,
			opts.MonitoringServiceURL,
			fmt.Sprintf(`histogram_quantile(0.99, 
				sum by (le, pod) (
					rate(etcd_disk_backend_commit_duration_seconds_bucket{namespace='%s'}[5m])
				)
			)`, opts.EtcdNamespace),
		},
		{
			// rate of WAL write bytes
			"promtool",
			"query",
			"instant",
			"-o",
			opts.MonitoringOutputFormat,
			opts.MonitoringServiceURL,
			fmt.Sprintf(`sum by (pod) (
				rate(etcd_disk_wal_write_bytes_total{namespace='%s'}[5m])
			)`, opts.EtcdNamespace),
		},
		{
			// p99 peer round-trip time
			// query for ROUND_TRIPPER_RAFT_MESSAGE connection type to fetch small
			// heartbeat/consensus traffic which dictates election-timeout risk
			"promtool",
			"query",
			"instant",
			"-o",
			opts.MonitoringOutputFormat,
			opts.MonitoringServiceURL,
			fmt.Sprintf(`histogram_quantile(
				0.99,
				sum by (le, pod, To) (
					rate(etcd_network_peer_round_trip_time_seconds_bucket{namespace='%s', ConnectionType='ROUND_TRIPPER_RAFT_MESSAGE'}[5m])
				)
			)`, opts.EtcdNamespace),
		},
		{
			// peer send failure rates for multi-node cluster
			// use the 'To' group-by to obtain the per peer node rates, instead of the
			// cluster-wide rate
			"promtool",
			"query",
			"instant",
			"-o",
			opts.MonitoringOutputFormat,
			opts.MonitoringServiceURL,
			fmt.Sprintf(`sum by (pod, To) (
				rate(etcd_network_peer_sent_failures_total{namespace="%s"}[5m])
				)`, opts.EtcdNamespace),
		},
		{
			// peer receive failure rates for multi-node cluster
			// use the 'To' group-by to obtain the per peer node rates, instead of the
			// cluster-wide rate
			"promtool",
			"query",
			"instant",
			"-o",
			opts.MonitoringOutputFormat,
			opts.MonitoringServiceURL,
			fmt.Sprintf(`sum by (pod, To) (
				rate(etcd_network_peer_received_failures_total{namespace="%s"}[5m])
				)`, opts.EtcdNamespace),
		},
	}

	runCmds := [][]string{}
	for _, cmd := range cmds {
		cmd = append(cmd, args...)
		runCmds = append(runCmds, cmd)
	}
	r, err := k8s.ExecPod(ctx, s.Clients, pod, runCmds)
	// don't discard any partial outputs
	if r != nil {
		b, readErr := io.ReadAll(r)
		return b, runCmds, errors.Join(err, readErr)
	}
	return nil, runCmds, err
}

func (s *BenchmarkSuite) SetClients(clients *pkgsuites.Clients) {
	s.Clients = clients
}

type BenchmarkOptions struct {
	EtcdBenchmarkLocalPath string
	EtcdctlLocalPath       string
	PromtoolLocalPath      string

	EtcdctlOutputFormat     string
	EtcdEndpoints           string
	EtcdMetricsPath         string
	EtcdMetricsPortName     string
	EtcdMetricsScheme       string
	EtcdNamespace           string
	EtcdRemoteTLSCertDir    string
	EtcdRemoteCopyTargetDir string

	JobActiveDeadline      time.Duration
	JobPodContainerName    string
	JobPodImageName        string
	JobPodImageTag         string
	JobPodNode             string
	JobPodTTLAfterFinished time.Duration
	JobPodReadyTimeout     time.Duration
	JobSuspend             bool

	MonitoringAddonName             string
	MonitoringNamespace             string
	MonitoringServiceURL            string
	MonitoringOutputFormat          string
	MonitoringWaitPodMonitorTimeout time.Duration
	MonitoringScrapeInterval        time.Duration

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
		EtcdBenchmarkLocalPath: "/usr/local/bin/benchmark",
		EtcdctlLocalPath:       "/usr/local/bin/etcdctl",
		PromtoolLocalPath:      "/usr/local/bin/promtool",

		EtcdctlOutputFormat:     "simple",
		EtcdEndpoints:           "https://127.0.0.1:2379",
		EtcdMetricsPath:         "/metrics",
		EtcdMetricsPortName:     "metrics",
		EtcdMetricsScheme:       "http",
		EtcdNamespace:           sysOpts.EtcdNamespace,
		EtcdRemoteCopyTargetDir: "/usr/local/bin/",
		EtcdRemoteTLSCertDir:    "/host/rancher/rke2/server/tls/etcd",

		JobActiveDeadline:      sysOpts.JobActiveDeadline,
		JobPodContainerName:    sysOpts.JobPodContainerName,
		JobPodImageName:        sysOpts.JobPodImageName,
		JobPodImageTag:         sysOpts.JobPodImageTag,
		JobPodNode:             sysOpts.JobPodNode,
		JobPodTTLAfterFinished: sysOpts.JobPodTTLAfterFinished,
		JobPodReadyTimeout:     sysOpts.JobPodReadyTimeout,
		JobSuspend:             sysOpts.JobSuspend,

		MonitoringAddonName:             sysOpts.MonitoringAddonName,
		MonitoringNamespace:             sysOpts.MonitoringNamespace,
		MonitoringServiceURL:            sysOpts.MonitoringServiceURL,
		MonitoringOutputFormat:          "promql",
		MonitoringWaitPodMonitorTimeout: sysOpts.MonitoringWaitPodMonitorTimeout,
		MonitoringScrapeInterval:        sysOpts.MonitoringScrapeInterval,

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
