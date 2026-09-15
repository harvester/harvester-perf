package etcd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/harvester/hvperf/internal/suites/options"
	"github.com/harvester/hvperf/pkg/k8s"
	prom "github.com/harvester/hvperf/pkg/prometheus"
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
	*pkgsuites.ProgressReporter
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
	//      return pkgsuites.SuiteResult{}, err
	// }

	etcd, etcdReady, err := k8s.EnsureEtcdReady(ctx, s.Clients, o.EtcdNamespace, o.EtcdReadyTimeout)
	if err != nil {
		return pkgsuites.SuiteResult{}, fmt.Errorf("failed to ensure etcd pods are ready: %w", err)
	}
	if !etcdReady {
		return pkgsuites.SuiteResult{}, fmt.Errorf("etcd pods are not ready in namespace '%s'", o.EtcdNamespace)
	}
	klog.V(3).InfoS("etcd pods are ready", "namespace", o.EtcdNamespace, "count", len(etcd.Items))

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

	var caseResults []*pkgsuites.CaseResult

	// issue exec command to run the etcdctl tool in the job pod
	klog.V(3).Infof("running etcdctl healthcheck in pod '%s'\n", pod.GetName())
	s.CaseStart(s.Name(), "etcd healthcheck")
	start := time.Now()
	results := s.execHealthcheck(ctx, pod, o, s.args(o)...)
	caseResults = append(caseResults, &pkgsuites.CaseResult{
		CaseName:      "etcd healthcheck",
		CmdResults:    results,
		DateTimeStart: start,
		DateTimeEnd:   time.Now(),
		Objects:       []runtime.Object{job, pod},
	})
	s.CaseDone(s.Name(), "etcd healthcheck", err == nil, time.Since(start))

	klog.V(3).Infof("running etcdctl check perf in pod '%s'\n", pod.GetName())
	s.CaseStart(s.Name(), "etcd check perf")
	start = time.Now()
	results = s.execCheckPerf(ctx, pod, o, s.args(o)...)
	caseResults = append(caseResults, &pkgsuites.CaseResult{
		CaseName:      "etcd check perf",
		CmdResults:    results,
		DateTimeStart: start,
		DateTimeEnd:   time.Now(),
		Objects:       []runtime.Object{job, pod},
	})
	s.CaseDone(s.Name(), "etcd check perf", err == nil, time.Since(start))

	// issue exec commands to run the benchmark tool in the job pod
	klog.V(3).Infof("running etcd benchmark (serial) in pod '%s'\n", pod.GetName())
	s.CaseStart(s.Name(), "etcd benchmark (serial)")
	start = time.Now()
	results = s.execBenchmark(ctx, pod, o, s.args(o)...)
	caseResults = append(caseResults, &pkgsuites.CaseResult{
		CaseName:      "etcd benchmark (serial)",
		CmdResults:    results,
		DateTimeStart: start,
		DateTimeEnd:   time.Now(),
		Objects:       []runtime.Object{job, pod},
	})
	s.CaseDone(s.Name(), "etcd benchmark (serial)", err == nil, time.Since(start))

	klog.V(3).Infof("running etcd benchmark (concurrent) in pod '%s'\n", pod.GetName())
	s.CaseStart(s.Name(), "etcd benchmark (concurrent)")
	start = time.Now()
	o.PutLoadSize = DefaultConcurrentLoadSize
	o.GRPCClientCount = DefaultConcurrentClientCount
	o.GRPCConnCount = DefaultConcurrentConnCount
	results = s.execBenchmark(ctx, pod, o, s.args(o)...)
	caseResults = append(caseResults, &pkgsuites.CaseResult{
		CaseName:      "etcd benchmark (concurrent)",
		CmdResults:    results,
		DateTimeStart: start,
		DateTimeEnd:   time.Now(),
		Objects:       []runtime.Object{job, pod},
	})
	s.CaseDone(s.Name(), "etcd benchmark (concurrent)", err == nil, time.Since(start))

	klog.V(3).Infof("running etcd monitoring (promql) in pod '%s'\n", pod.GetName())
	s.CaseStart(s.Name(), "etcd monitoring (promql)")
	start = time.Now()
	metricResults, skipped, err := s.monitoring(ctx, pod, len(etcd.Items), o)
	caseResults = append(caseResults, &pkgsuites.CaseResult{
		CaseName:      "etcd monitoring (promql)",
		DateTimeStart: start,
		DateTimeEnd:   time.Now(),
		Err:           err,
		Objects:       []runtime.Object{job, pod},
		MetricResults: metricResults,
		Skipped:       skipped,
	})
	s.CaseDone(s.Name(), "etcd monitoring (promql)", err == nil, time.Since(start))

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
) []*pkgsuites.CmdResult {
	outArgs := []string{
		"-w", opts.EtcdctlOutputFormat,
	}
	cmds := [][]string{
		{"etcdctl", "endpoint", "status"},
		{"etcdctl", "endpoint", "health"},
		{"etcdctl", "member", "list"},
	}

	var results []*pkgsuites.CmdResult
	args = append(args, outArgs...)
	for _, cmd := range cmds {
		cmd = append(cmd, args...)
		out, err := k8s.ExecPod(ctx, s.Clients, pod, cmd)
		result := &pkgsuites.CmdResult{
			Cmd:    strings.Join(cmd, " "),
			Err:    err,
			Stdout: out.Stdout,
			Stderr: out.Stderr,
		}
		results = append(results, result)
	}
	return results
}

func (s *BenchmarkSuite) execCheckPerf(
	ctx context.Context,
	pod *corev1.Pod,
	opts *BenchmarkOptions,
	args ...string,
) []*pkgsuites.CmdResult {
	outArgs := []string{
		"-w", opts.EtcdctlOutputFormat,
		"--load", opts.CheckPerfLoadSize,
	}
	cmds := [][]string{
		{"etcdctl", "check", "perf"},
	}

	var results []*pkgsuites.CmdResult
	args = append(args, outArgs...)
	for _, cmd := range cmds {
		cmd = append(cmd, args...)
		out, err := k8s.ExecPod(ctx, s.Clients, pod, cmd)
		result := &pkgsuites.CmdResult{
			Cmd:    strings.Join(cmd, " "),
			Err:    err,
			Stdout: out.Stdout,
			Stderr: out.Stderr,
		}
		results = append(results, result)
	}
	return results
}

func (s *BenchmarkSuite) execBenchmark(
	ctx context.Context,
	pod *corev1.Pod,
	opts *BenchmarkOptions,
	args ...string,
) []*pkgsuites.CmdResult {
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

	var results []*pkgsuites.CmdResult
	for _, cmd := range cmds {
		cmd = append(cmd, args...)
		out, err := k8s.ExecPod(ctx, s.Clients, pod, cmd)
		result := &pkgsuites.CmdResult{
			Cmd:    strings.Join(cmd, " "),
			Err:    err,
			Stdout: out.Stdout,
			Stderr: out.Stderr,
		}
		results = append(results, result)
	}
	return results
}

func (s *BenchmarkSuite) monitoring(
	ctx context.Context,
	pod *corev1.Pod,
	etcdCount int,
	opts *BenchmarkOptions,
) ([]*pkgsuites.MetricResult, bool, error) {
	// check if monitoring addon is enabled and ready. if not, skip the promql
	// execution.
	ready, err := k8s.MonitoringEnabled(ctx, s.Clients, opts.MonitoringNamespace, opts.MonitoringAddonName)
	if err != nil {
		klog.V(3).ErrorS(err, "failed to check if monitoring addon is enabled, skipping etcd promql test case", "namespace", opts.MonitoringNamespace, "addon", opts.MonitoringAddonName)
		return nil, true, nil
	}
	if !ready {
		klog.V(3).InfoS("monitoring addon is not enabled in namespace, skipping etcd promql test case", "namespace", opts.MonitoringNamespace, "addon", opts.MonitoringAddonName)
		return nil, true, nil
	}

	// monitoring addon is enabled, so all errors from this point on should be
	// returned to the caller, as they indicate a failure in the test case.

	// etcd metrics are not exposed by default, so we need to ensure that the pod
	// monitor is created
	podMonOpts := &k8s.PodMonitorOption{
		Name:            s.Name(),
		Namespace:       pod.GetNamespace(),
		EndpointScheme:  opts.EtcdMetricsScheme,
		EtcdCount:       etcdCount,
		LabelSelector:   k8s.EtcdLabelSelector.MatchLabels,
		MetricsPortName: opts.EtcdMetricsPortName,
		MetricsPath:     opts.EtcdMetricsPath,
		RangeDuration:   opts.MonitoringRangeDuration,
		TargetNamespace: opts.EtcdNamespace,
		WaitTimeout:     opts.MonitoringWaitPodMonitorTimeout,
	}
	cleanup, podMonErr := k8s.EnsurePodMonitor(
		ctx,
		s.Clients,
		podMonOpts,
		pod,
	)
	defer func() {
		klog.V(3).InfoS("cleaning up pod monitor", "name", podMonOpts.Name, "namespace", podMonOpts.Namespace)
		if err := cleanup(); err != nil {
			klog.V(3).ErrorS(err, "failed to cleanup pod monitor", "name", podMonOpts.Name, "namespace", podMonOpts.Namespace)
		}
	}()
	if podMonErr != nil {
		return nil, true, podMonErr
	}
	klog.V(3).InfoS("pod monitor is ready", "name", podMonOpts.Name, "namespace", podMonOpts.Namespace)

	metricResults, err := s.execPromQL(ctx, opts)
	if err != nil {
		return nil, false, err
	}

	return metricResults, false, err
}

func (s *BenchmarkSuite) execPromQL(
	ctx context.Context,
	opts *BenchmarkOptions,
) ([]*pkgsuites.MetricResult, error) {
	queries := []string{
		// p99 WAL fsync
		fmt.Sprintf(`histogram_quantile(0.99,sum by (le, pod) (rate(etcd_disk_wal_fsync_duration_seconds_bucket{namespace='%s'}[%s])))`, opts.EtcdNamespace, opts.MonitoringRangeDuration),

		// p99 backend commit
		fmt.Sprintf(`histogram_quantile(0.99,sum by (le, pod) (rate(etcd_disk_backend_commit_duration_seconds_bucket{namespace='%s'}[%s])))`, opts.EtcdNamespace, opts.MonitoringRangeDuration),

		// rate of WAL write bytes
		fmt.Sprintf(`sum by (pod) (rate(etcd_disk_wal_write_bytes_total{namespace='%s'}[%s]))`, opts.EtcdNamespace, opts.MonitoringRangeDuration),

		// p99 peer round-trip time
		// query for ROUND_TRIPPER_RAFT_MESSAGE connection type to fetch small
		// heartbeat/consensus traffic which dictates election-timeout risk
		fmt.Sprintf(`histogram_quantile(0.99,sum by (le, pod, To) (rate(etcd_network_peer_round_trip_time_seconds_bucket{namespace='%s', ConnectionType='ROUND_TRIPPER_RAFT_MESSAGE'}[%s])))`, opts.EtcdNamespace, opts.MonitoringRangeDuration),

		// peer send failure rates for multi-node cluster
		// use the 'To' group-by to obtain the per peer node rates, instead of the
		// cluster-wide rate
		fmt.Sprintf(`sum by (pod, To) (rate(etcd_network_peer_sent_failures_total{namespace="%s"}[%s]))`, opts.EtcdNamespace, opts.MonitoringRangeDuration),

		// peer receive failure rates for multi-node cluster
		// use the 'From' group-by to obtain the per peer node rates, instead of the
		// cluster-wide rate
		fmt.Sprintf(`sum by (pod, From) (rate(etcd_network_peer_received_failures_total{namespace="%s"}[%s]))`, opts.EtcdNamespace, opts.MonitoringRangeDuration),
	}

	var (
		metricResults []*pkgsuites.MetricResult
		errs          error
	)
	for _, query := range queries {
		v, _, err := prom.RunInstant(ctx, s.PromClient, query)
		result := &pkgsuites.MetricResult{
			Query:   query,
			Samples: v,
		}
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to execute promql query '%s': %w", query, err))
		}
		metricResults = append(metricResults, result)
	}
	return metricResults, errs
}

func (s *BenchmarkSuite) SetClients(clients *pkgsuites.Clients) {
	s.Clients = clients
}

func (s *BenchmarkSuite) SetProgressReporter(progressReporter *pkgsuites.ProgressReporter) {
	s.ProgressReporter = progressReporter
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
	MonitoringRangeDuration time.Duration
	EtcdNamespace           string
	EtcdReadyTimeout        time.Duration
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
	MonitoringOutputFormat          string
	MonitoringWaitPodMonitorTimeout time.Duration

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
		EtcdReadyTimeout:        sysOpts.EtcdReadyTimeout,
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
		MonitoringRangeDuration:         sysOpts.MonitoringRangeDuration,
		MonitoringOutputFormat:          "promql",
		MonitoringWaitPodMonitorTimeout: sysOpts.MonitoringWaitPodMonitorTimeout,

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
