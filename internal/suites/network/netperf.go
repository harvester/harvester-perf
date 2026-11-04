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

func (s *NetworkSuite) netperf(
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
	serverCmdResults, serverJob, serverPod, err := s.netperfServer(
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
	serverIP, err := podIPByIPFamily(serverPod, o.NetPerfIPFamily)
	if err != nil {
		return suites.NewCaseResult(caseName, start, time.Now(), cmdResults, nil, objs...).
			WithError(err)
	}

	clientCmdResults, clientJob, clientPod, err := s.netperfClient(
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
	return suites.NewCaseResult(caseName, start, time.Now(), cmdResults, metricResults, objs...)
}

func (s *NetworkSuite) netperfServer(
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

func (s *NetworkSuite) netperfClient(
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

type NetworkOptions struct {
	JobActiveDeadline      time.Duration
	JobPodImageName        string
	JobPodImageTag         string
	JobPodNode             string
	JobPodTTLAfterFinished time.Duration
	JobPodReadyTimeout     time.Duration
	JobSuspend             bool
	NamespaceReadyTimeout  time.Duration

	IPerfBinLocalPath           string
	IPerfClientMessageSizeBytes int
	IPerfClientParallelStreams  int
	IPerfIPFamily               IPFamilyOption
	IPerfRemoteCopyTargetDir    string
	IPerfServerPort             int
	IPerfTestDurationSeconds    int

	NetPerfIPFamily             IPFamilyOption
	NetPerfClientLocalPath      string
	NetPerfServerLocalPath      string
	NetPerfRemoteCopyTargetDir  string
	NetPerfTestStartWaitSeconds int
	NetPerfTestLengthSeconds    int
}

func NewNetworkOptions(override suites.Options) (*NetworkOptions, error) {
	merged := options.DefaultGlobalOptions().
		Merge(options.DefaultNetworkOptions()).
		Merge(override)

	// if user did not provide the netperf or iperf client/server local paths,
	// use the toolsDir option to construct them. if toolsDir is not provided,
	// return an error.
	if merged["netPerfClientLocalPath"] == nil ||
		merged["netPerfServerLocalPath"] == nil ||
		merged["iperfBinLocalPath"] == nil {
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
		if merged["iperfBinLocalPath"] == nil {
			merged["iperfBinLocalPath"] = filepath.Join(cast, "iperf3")
		}
	}
	return options.FromOptions[*NetworkOptions](merged)
}

func podIPByIPFamily(pod *corev1.Pod, ipFamily IPFamilyOption) (string, error) {
	var serverIP string
LOOP:
	for _, ip := range pod.Status.PodIPs {
		addr, err := netip.ParseAddr(ip.IP)
		if err != nil {
			return "", fmt.Errorf("failed to parse server pod IP '%s': %w", ip.IP, err)
		}

		switch {
		case ipFamily == IPv4 && addr.Is4():
			serverIP = ip.IP
			break LOOP
		case ipFamily == IPv6 && addr.Is6():
			serverIP = ip.IP
			break LOOP
		}
	}

	if serverIP == "" {
		return "", fmt.Errorf("no server pod IP found for requested IP family '%s'", ipFamily)
	}

	return serverIP, nil
}
