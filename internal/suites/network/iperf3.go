package network

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/harvester/hvperf/pkg/k8s"
	"github.com/harvester/hvperf/pkg/suites"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
)

func (s *NetworkSuite) iperf3(
	ctx context.Context,
	caseName string,
	runID string,
	namespace string,
	profile ProfileName,
	o *NetworkOptions,
) *suites.CaseResult {
	var (
		cmdResults []*suites.CmdResult
		objs       []runtime.Object
	)
	start := time.Now()
	serverCmdResults, serverJob, serverPod, err := s.iperf3Server(
		ctx,
		runID,
		namespace,
		profile,
		o,
	)
	if err != nil {
		return suites.NewCaseResultErrored(caseName, start, time.Now(), fmt.Errorf("failed to setup iperf server job: %w", err))
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
	serverIP, err := podIPByIPFamily(serverPod, o.IPerfIPFamily)
	if err != nil {
		return suites.NewCaseResult(caseName, start, time.Now(), cmdResults, nil, objs...).
			WithError(err)
	}

	clientCmdResults, clientJob, clientPod, err := s.iperf3Client(
		ctx,
		runID,
		namespace,
		serverIP,
		profile,
		o,
	)
	if err != nil {
		return suites.NewCaseResult(caseName, start, time.Now(), cmdResults, nil, objs...).
			WithError(fmt.Errorf("failed to setup iperf client job: %w", err))
	}
	cmdResults = append(cmdResults, clientCmdResults...)
	objs = append(objs, clientJob, clientPod)
	return suites.NewCaseResult(caseName, start, time.Now(), cmdResults, nil, objs...)
}

func (s *NetworkSuite) iperf3Server(
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
	job, pod, err := s.iperf3Setup(ctx, runID, namespace, profile, BenchmarkServer, o)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to setup iperf server job: %w", err)
	}

	timeout := 60 * time.Second
	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := []string{"iperf3", "-s", "-D", "-p", strconv.Itoa(o.IPerfServerPort)}
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

func (s *NetworkSuite) iperf3Client(
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
	job, pod, err := s.iperf3Setup(ctx, runID, namespace, profile, BenchmarkClient, o)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to setup iperf client job: %w", err)
	}

	timeout := time.Duration(o.IPerfTestDurationSeconds*12) * time.Second
	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := []string{
		"iperf3",
		string(o.IPerfIPFamily),
		"-P", strconv.Itoa(o.IPerfClientParallelStreams),
		"-c", serverIP,
		"-t", strconv.Itoa(o.IPerfTestDurationSeconds),
		"-l", strconv.Itoa(o.IPerfClientMessageSizeBytes),
		"-p", strconv.Itoa(o.IPerfServerPort),
	}
	if profile == ProfileUDPStream {
		cmd = append(cmd, "-u", "-b", "0")
	}
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

func (s *NetworkSuite) iperf3Setup(
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
	overrides := func(job *batchv1.Job) error {
		job.Labels["harvesterhci.io/iperf-workload-type"] = string(workloadType)
		job.Labels["harvesterhci.io/iperf-profile"] = string(profile)
		job.Spec.Template.Spec.HostNetwork = false
		if job.Spec.Template.Labels == nil {
			job.Spec.Template.Labels = map[string]string{}
		}
		job.Spec.Template.Labels["harvesterhci.io/iperf-workload-type"] = string(workloadType)
		job.Spec.Template.Labels["harvesterhci.io/iperf-profile"] = string(profile)
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

	klog.V(3).Infof("copying '%s' to pod '%s/%s'\n", o.IPerfBinLocalPath, pod.GetNamespace(), pod.GetName())
	if err := k8s.CopyToJobPod(
		ctx,
		s.Clients,
		pod,
		o.IPerfRemoteCopyTargetDir,
		o.IPerfBinLocalPath,
	); err != nil {
		return nil, nil, err
	}
	return job, pod, nil
}
