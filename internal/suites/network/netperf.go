package network

import (
	"context"
	"fmt"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/harvester/hvperf/internal/suites/options"
	"github.com/harvester/hvperf/pkg/k8s"
	"github.com/harvester/hvperf/pkg/suites"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
)

func init() {
	suites.Register(NewNetworkSuite())
}

var _ suites.Suite = &NetworkSuite{}

type NetworkSuite struct {
	suites.SuiteMarshaler
	*suites.ProgressReporter
	*suites.Clients
}

func NewNetworkSuite() *NetworkSuite {
	n := &NetworkSuite{}
	n.Marshal = n
	return n
}

func (s *NetworkSuite) Name() string {
	return "network-benchmark"
}

func (s *NetworkSuite) Description() string {
	return "run netperf to benchmark the in-cluster network latency and throughput"
}

func (s *NetworkSuite) IsExclusive() bool {
	return true
}

func (s *NetworkSuite) SetClients(clients *suites.Clients) {
	s.Clients = clients
}

func (s *NetworkSuite) SetProgressReporter(reporter *suites.ProgressReporter) {
	s.ProgressReporter = reporter
}

func (s *NetworkSuite) RunE(
	ctx context.Context,
	runID string,
	namespace string,
	opts suites.Options,
) suites.SuiteResult {
	o, err := NewNetPerfOptions(opts)
	if err != nil {
		return suites.NewSuiteResult(s.Name(), runID).WithError(err)
	}

	if _, err := k8s.EnsureNamespace(ctx, s.Clients, namespace, o.NamespaceReadyTimeout); err != nil {
		return suites.SuiteResult{
			Name:  s.Name(),
			RunID: runID,
			Err:   fmt.Sprintf("namespace %s not ready: %v", namespace, err.Error()),
		}
	}
	klog.V(3).InfoS("namespace is ready\n", "name", namespace)

	testSuites := []struct {
		name    string
		profile ProfileName
		fn      func(context.Context, string, string, string, ProfileName, *NetworkOptions) *suites.CaseResult
	}{
		{
			name:    fmt.Sprintf("netperf-%s", ProfileTCPStream),
			profile: ProfileTCPStream,
			fn:      s.execNetperf,
		},
		{
			name:    fmt.Sprintf("netperf-%s", ProfileTCPCRR),
			profile: ProfileTCPCRR,
			fn:      s.execNetperf,
		},
		{
			name:    fmt.Sprintf("netperf-%s", ProfileTCPRR),
			profile: ProfileTCPRR,
			fn:      s.execNetperf,
		},
		{
			name:    fmt.Sprintf("netperf-%s", ProfileUDPStream),
			profile: ProfileUDPStream,
			fn:      s.execNetperf,
		},
		{
			name:    fmt.Sprintf("netperf-%s", ProfileUDPRR),
			profile: ProfileUDPRR,
			fn:      s.execNetperf,
		},
		{
			name: "network-benchmark-iperf3",
			fn:   s.iperf,
		},
	}

	var caseResults []*suites.CaseResult
	for _, testCase := range testSuites {
		s.CaseStart(s.Name(), testCase.name)
		caseResult := testCase.fn(ctx, testCase.name, runID, namespace, testCase.profile, o)
		caseResults = append(caseResults, caseResult)
		s.CaseDone(s.Name(), testCase.name, caseResult.State, time.Since(caseResult.DateTimeStart))
	}

	suiteParams, err := suites.ToSuiteParams(o)
	if err != nil {
		// just log the params conversion error, don't fail the suite. the only effect
		// is that the suite params won't be reported in the results.
		klog.V(3).ErrorS(err, "failed to convert suite options to suite params\n")
	}

	return suites.NewSuiteResult(s.Name(), runID).WithParams(suiteParams).WithResults(caseResults)
}

func (s *NetworkSuite) execNetperf(
	ctx context.Context,
	caseName string,
	runID string,
	namespace string,
	profile ProfileName,
	o *NetworkOptions,
) *suites.CaseResult {
	var (
		cmdResults    []*suites.CmdResult
		metricResults []*suites.MetricResult
		objs          []runtime.Object
	)
	start := time.Now()
	serverCmdResults, serverJob, serverPod, err := s.execNetperfServer(
		ctx,
		runID,
		namespace,
		profile,
		o,
	)
	if err != nil {
		return suites.NewCaseResultErrored(caseName, start, time.Now(), fmt.Errorf("failed to setup netperf server job: %w", err))
	}
	cmdResults = append(cmdResults, serverCmdResults...)
	objs = append(objs, serverJob, serverPod)

	// if the server job failed, we don't need to run the client job, so we can
	// return early with all the results we have so far.
	for _, r := range serverCmdResults {
		if r.Err != "" {
			return suites.NewCaseResult(caseName, start, time.Now(), cmdResults, nil, objs...)
		}
	}

	// look for the pod IP address that matches the requested IP family. if we can't
	// find one, return an error.
	serverIP, err := podIPByIPFamily(serverPod, o)
	if err != nil {
		return suites.NewCaseResult(caseName, start, time.Now(), cmdResults, nil, objs...).
			WithError(err)
	}

	clientCmdResults, clientJob, clientPod, err := s.execNetperfClient(
		ctx,
		runID,
		namespace,
		serverIP,
		profile,
		o,
	)
	if err != nil {
		return suites.NewCaseResult(caseName, start, time.Now(), cmdResults, nil, objs...).
			WithError(fmt.Errorf("failed to setup netperf client job: %w", err))
	}
	cmdResults = append(cmdResults, clientCmdResults...)
	objs = append(objs, clientJob, clientPod)

	serverVersionCmdResult := s.execNetperfVersion(ctx, serverPod, BenchmarkServer)
	cmdResults = append(cmdResults, serverVersionCmdResult)

	clientVersionCmdResult := s.execNetperfVersion(ctx, clientPod, BenchmarkClient)
	cmdResults = append(cmdResults, clientVersionCmdResult)

	return suites.NewCaseResult(caseName, start, time.Now(), cmdResults, metricResults, objs...)
}

func (s *NetworkSuite) execNetperfServer(
	ctx context.Context,
	runID string,
	namespace string,
	profile ProfileName,
	o *NetworkOptions,
) (
	[]*suites.CmdResult,
	*batchv1.Job,
	*corev1.Pod,
	error,
) {
	job, pod, err := s.netperfSetup(ctx, runID, namespace, profile, BenchmarkServer, o)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to setup netperf server job: %w", err)
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, time.Duration(3*(o.NetPerfTestLengthSeconds+o.NetPerfTestStartWaitSeconds))*time.Second)
	defer cancel()

	cmd := []string{"netserver", "-d", string(o.NetPerfIPFamily)}
	out, err := k8s.ExecPod(ctxWithTimeout, s.Clients, pod, cmd)
	cmdResult := &suites.CmdResult{
		Cmd:    strings.Join(cmd, " "),
		Stdout: out.Stdout,
		Stderr: out.Stderr,
	}
	if err != nil {
		cmdResult.Err = err.Error()
	}

	return []*suites.CmdResult{cmdResult}, job, pod, nil
}

func (s *NetworkSuite) execNetperfClient(
	ctx context.Context,
	runID string,
	namespace string,
	serverIP string,
	profile ProfileName,
	o *NetworkOptions,
) (
	[]*suites.CmdResult,
	*batchv1.Job,
	*corev1.Pod,
	error,
) {
	job, pod, err := s.netperfSetup(ctx, runID, namespace, profile, BenchmarkClient, o)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to setup netperf client job: %w", err)
	}

	timeout := 3 * (o.NetPerfTestLengthSeconds + o.NetPerfTestStartWaitSeconds)
	ctxWithTimeout, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := []string{
		"netperf",
		"-s", strconv.Itoa(o.NetPerfTestStartWaitSeconds),
		"-H", serverIP,
		"-l", strconv.Itoa(o.NetPerfTestLengthSeconds),
		"-t", string(profile),
		"--",
	}
	cmd = append(cmd, ProfileParams[profile]...)
	out, err := k8s.ExecPod(ctxWithTimeout, s.Clients, pod, cmd)
	cmdResult := &suites.CmdResult{
		Cmd:    strings.Join(cmd, " "),
		Stdout: out.Stdout,
		Stderr: out.Stderr,
	}
	if err != nil {
		cmdResult.Err = err.Error()
	}
	return []*suites.CmdResult{cmdResult}, job, pod, nil
}

func (s *NetworkSuite) execNetperfVersion(
	ctx context.Context,
	pod *corev1.Pod,
	workloadType BenchmarkWorkloadType,
) *suites.CmdResult {
	var cmd []string
	switch workloadType {
	case BenchmarkServer:
		cmd = []string{"netserver", "-V"}
	case BenchmarkClient:
		cmd = []string{"netperf", "-V"}
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	out, err := k8s.ExecPod(ctxWithTimeout, s.Clients, pod, cmd)
	cmdResult := &suites.CmdResult{
		Cmd:    strings.Join(cmd, " "),
		Stdout: out.Stdout,
		Stderr: out.Stderr,
	}
	if err != nil {
		cmdResult.Err = err.Error()
	}
	return cmdResult
}

func (s *NetworkSuite) netperfSetup(
	ctx context.Context,
	runID string,
	namespace string,
	profile ProfileName,
	workloadType BenchmarkWorkloadType,
	o *NetworkOptions,
) (
	*batchv1.Job,
	*corev1.Pod,
	error,
) {
	var netperfBin string
	switch workloadType {
	case BenchmarkServer:
		netperfBin = o.NetPerfServerLocalPath
	case BenchmarkClient:
		netperfBin = o.NetPerfClientLocalPath
	}

	overrides := func(job *batchv1.Job) error {
		job.Labels["harvesterhci.io/netperf-workload-type"] = string(workloadType)
		job.Labels["harvesterhci.io/netperf-profile"] = string(profile)
		job.Spec.Template.Spec.HostNetwork = false
		if job.Spec.Template.Labels == nil {
			job.Spec.Template.Labels = map[string]string{}
		}
		job.Spec.Template.Labels["harvesterhci.io/netperf-workload-type"] = string(workloadType)
		job.Spec.Template.Labels["harvesterhci.io/netperf-profile"] = string(profile)
		return nil
	}

	job, pod, err := k8s.EnsureJobReady(
		ctx,
		s.Clients,
		s.Name(),
		runID,
		namespace,
		o.JobPodNode,
		o.JobPodImageName+":"+o.JobPodImageTag,
		o.JobActiveDeadline,
		o.JobPodReadyTimeout,
		o.JobPodTTLAfterFinished,
		o.JobSuspend,
		overrides,
	)
	if err != nil {
		return nil, nil, err
	}
	klog.V(3).Infof("pod:'%s/%s' is now ready, phase:'%s'\n", pod.GetNamespace(), pod.GetName(), pod.Status.Phase)

	klog.V(3).Infof("copying '%s' to pod '%s/%s'\n", netperfBin, pod.GetNamespace(), pod.GetName())
	if err := k8s.CopyToJobPod(
		ctx,
		s.Clients,
		pod,
		o.NetPerfRemoteCopyTargetDir,
		netperfBin,
	); err != nil {
		return nil, nil, err
	}
	return job, pod, nil
}

func (s *NetworkSuite) iperf(
	ctx context.Context,
	caseName string,
	runID string,
	namespace string,
	testName ProfileName,
	o *NetworkOptions,
) *suites.CaseResult {
	start := time.Now()
	var (
		cmdResults    []*suites.CmdResult
		metricResults []*suites.MetricResult
		objs          []runtime.Object
	)
	end := time.Now()
	return suites.NewCaseResult(caseName, start, end, cmdResults, metricResults, objs...)
}

type NetworkOptions struct {
	JobActiveDeadline      time.Duration
	JobPodImageName        string
	JobPodImageTag         string
	JobPodNode             string
	JobPodTTLAfterFinished time.Duration
	JobPodReadyTimeout     time.Duration
	JobSuspend             bool
	NamespaceReadyTimeout  time.Duration

	NetPerfIPFamily             IPFamilyOption
	NetPerfClientLocalPath      string
	NetPerfServerLocalPath      string
	NetPerfRemoteCopyTargetDir  string
	NetPerfTestStartWaitSeconds int
	NetPerfTestLengthSeconds    int
}

func NewNetPerfOptions(override suites.Options) (*NetworkOptions, error) {
	merged := options.DefaultGlobalOptions().
		Merge(options.DefaultNetworkOptions()).
		Merge(override)

	// if user did not provide the netperf client/server local paths,
	// use the toolsDir option to construct them. if toolsDir is not provided,
	// return an error.
	if merged["netPerfClientLocalPath"] == nil || merged["netPerfServerLocalPath"] == nil {
		toolsDir, exists := merged["toolsDir"]
		if !exists {
			return nil, fmt.Errorf("toolsDir option is required but not provided")
		}

		cast, ok := toolsDir.(string)
		if !ok {
			return nil, fmt.Errorf("toolsDir option must be a string, but got %T", toolsDir)
		}

		if merged["netPerfClientLocalPath"] == nil {
			merged["netPerfClientLocalPath"] = filepath.Join(cast, "netperf")
		}
		if merged["netPerfServerLocalPath"] == nil {
			merged["netPerfServerLocalPath"] = filepath.Join(cast, "netserver")
		}
	}
	return options.FromOptions[*NetworkOptions](merged)
}

func podIPByIPFamily(pod *corev1.Pod, o *NetworkOptions) (string, error) {
	var serverIP string
LOOP:
	for _, ip := range pod.Status.PodIPs {
		addr, err := netip.ParseAddr(ip.IP)
		if err != nil {
			return "", fmt.Errorf("failed to parse server pod IP '%s': %w", ip.IP, err)
		}

		switch {
		case o.NetPerfIPFamily == IPv4 && addr.Is4():
			serverIP = ip.IP
			break LOOP
		case o.NetPerfIPFamily == IPv6 && addr.Is6():
			serverIP = ip.IP
			break LOOP
		}
	}

	if serverIP == "" {
		return "", fmt.Errorf("no server pod IP found for requested IP family '%s'", o.NetPerfIPFamily)
	}

	return serverIP, nil
}

type (
	BenchmarkWorkloadType string

	// IPFamilyOption corresponds to the -4 or -6 option in netperf, which specifies
	// whether to use IPv4 or IPv6.
	IPFamilyOption string

	// ProfileName is the name of a netperf test profiles as defined in
	// https://hewlettpackard.github.io/netperf/doc/netperf.html#Options-common-to-TCP-UDP-and-SCTP-tests
	ProfileName string
)

const (
	BenchmarkClient BenchmarkWorkloadType = "client"
	BenchmarkServer BenchmarkWorkloadType = "server"

	IPv4 IPFamilyOption = "-4"
	IPv6 IPFamilyOption = "-6"

	ProfileTCPCRR    ProfileName = "TCP_CRR"
	ProfileTCPRR     ProfileName = "TCP_RR"
	ProfileTCPStream ProfileName = "TCP_STREAM"
	ProfileUDPRR     ProfileName = "UDP_RR"
	ProfileUDPStream ProfileName = "UDP_STREAM"
)

// ProfileParams are parameters for each test profile derived from k8s-netperf's
// implementation
var ProfileParams = map[ProfileName][]string{
	ProfileTCPCRR:    {"-k", "THROUGHPUT,THROUGHPUT_UNITS,MIN_LATENCY,MEAN_LATENCY,P50_LATENCY,P90_LATENCY,P99_LATENCY,MAX_LATENCY,STDDEV_LATENCY,LOCAL_TRANSPORT_RETRANS", "-r", "1024,1024", "-P", "42424"},
	ProfileTCPRR:     {"-k", "THROUGHPUT,THROUGHPUT_UNITS,MIN_LATENCY,MEAN_LATENCY,P50_LATENCY,P90_LATENCY,P99_LATENCY,MAX_LATENCY,STDDEV_LATENCY,LOCAL_TRANSPORT_RETRANS", "-r", "1024,1024", "-P", "42424"},
	ProfileTCPStream: {"-k", "THROUGHPUT,THROUGHPUT_UNITS,LOCAL_SEND_CALLS,REMOTE_RECV_CALLS,LOCAL_TRANSPORT_RETRANS", "-m", "1024", "-P", "42424"},
	ProfileUDPRR:     {"-k", "THROUGHPUT,THROUGHPUT_UNITS,MIN_LATENCY,MEAN_LATENCY,P50_LATENCY,P90_LATENCY,P99_LATENCY,MAX_LATENCY,STDDEV_LATENCY,LOCAL_TRANSPORT_RETRANS", "-r", "1024,1024", "-P", "42424"},
	ProfileUDPStream: {"-k", "THROUGHPUT,THROUGHPUT_UNITS,LOCAL_SEND_CALLS,REMOTE_RECV_CALLS,LOCAL_TRANSPORT_RETRANS", "-m", "1024", "-R", "1", "-P", "42424"},
}
