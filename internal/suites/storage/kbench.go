package storage

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/harvester/hvperf/internal/suites/options"
	"github.com/harvester/hvperf/pkg/k8s"
	prom "github.com/harvester/hvperf/pkg/prometheus"
	"github.com/harvester/hvperf/pkg/suites"

	"k8s.io/klog/v2"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func init() {
	suites.Register(NewKbenchSuite())
}

var _ suites.Suite = &KBenchSuite{}

type KBenchSuite struct {
	suites.SuiteMarshaler
	*suites.ProgressReporter
	*suites.Clients
}

func NewKbenchSuite() *KBenchSuite {
	k := &KBenchSuite{}
	k.Marshal = k
	return k
}

func (s *KBenchSuite) Name() string {
	return "storage-kbench"
}

func (s *KBenchSuite) Description() string {
	return "run kbench to benchmark the storage performance of the cluster"
}

func (s *KBenchSuite) IsReadWrite() bool {
	return true
}

func (s *KBenchSuite) RunE(
	ctx context.Context,
	runID string,
	namespace string,
	opts suites.Options,
) suites.SuiteResult {
	o, err := NewKBenchOptions(opts)
	if err != nil {
		return suites.SuiteResult{
			Name:  s.Name(),
			RunID: runID,
			Err:   err.Error(),
		}
	}

	if _, err := k8s.EnsureNamespace(ctx, s.Clients, namespace, o.NamespaceReadyTimeout); err != nil {
		return suites.SuiteResult{
			Name:  s.Name(),
			RunID: runID,
			Err:   fmt.Sprintf("namespace %s not ready: %v", namespace, err.Error()),
		}
	}
	klog.V(3).InfoS("namespace is ready\n", "name", namespace)

	var caseResults []*suites.CaseResult
	caseName := "kbench-pod-pvc-full"
	klog.V(3).InfoS("running kbench pod with pvc\n", "suite", s.Name(), "case", caseName)
	s.CaseStart(s.Name(), caseName)
	caseResult := s.kbenchPodPVC(ctx, caseName, runID, namespace, o)
	caseResults = append(caseResults, caseResult)
	s.CaseDone(s.Name(), caseName, caseResult.State, time.Since(caseResult.DateTimeStart))

	var (
		nodeName, pvcName string

		// the kbenchJobDuration is used to determine the promql query range. it is
		// calculated as the time since the pod was ready. it captures the time for the
		// kbench pod to finish its task and additional cleanup time. the overshoot is
		// acceptable to account for late-arriving scrape samples.
		kbenchJobDuration time.Duration
	)
	for _, obj := range caseResult.Objects {
		switch v := obj.(type) {
		case *corev1.Pod:
			nodeName = v.Spec.NodeName
			for _, condition := range v.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					kbenchJobDuration = time.Since(condition.LastTransitionTime.Time)
				}
			}
		case *corev1.PersistentVolumeClaim:
			pvcName = v.GetName()
		}
	}
	if nodeName != "" && pvcName != "" {
		caseName = caseName + "-monitoring"
		klog.V(3).InfoS("running monitoring pod on node\n", "suite", s.Name(), "case", caseName, "node", nodeName)
		s.CaseStart(s.Name(), caseName)
		caseResult = s.monitoring(ctx, caseName, namespace, nodeName, pvcName, kbenchJobDuration, o)
		caseResults = append(caseResults, caseResult)
		s.CaseDone(s.Name(), caseName, caseResult.State, time.Since(caseResult.DateTimeStart))
	}

	suiteParams, err := suites.ToSuiteParams(o)
	if err != nil {
		// just log the params conversion error, don't fail the suite. the only effect
		// is that the suite params won't be reported in the results.
		klog.V(3).ErrorS(err, "failed to convert suite options to suite params\n")
	}

	return suites.SuiteResult{
		Name:    s.Name(),
		Params:  suiteParams,
		RunID:   runID,
		Results: caseResults,
	}
}

func (s *KBenchSuite) kbenchPodPVC(
	ctx context.Context,
	caseName string,
	runID string,
	namespace string,
	opts *KBenchOptions,
) *suites.CaseResult {
	start := time.Now()
	pvc, pvcCleanup, err := k8s.CreatePVC(
		ctx,
		s.Clients,
		s.Name(),
		runID,
		namespace,
		opts.PVCStorageClassName,
		opts.PVCStorageSize,
	)
	defer func() {
		if pvcCleanup != nil {
			if err := pvcCleanup(); err != nil {
				klog.V(3).ErrorS(err, "failed to clean up pvc", "suite", s.Name(), "case", caseName)
			}
		}
	}()
	if err != nil {
		return suites.NewCaseResultErrored(s.Name(), start, time.Now(), fmt.Errorf("failed to create pvc: %v", err))
	}
	klog.V(3).InfoS("pvc is created\n", "name", pvc.Name, "namespace", pvc.Namespace)

	// job spec from https://github.com/longhorn/kbench/blob/main/deploy/fio.yaml
	overrides := func(job *batchv1.Job) error {
		overrideKBenchSpec(caseName, runID, job, opts)
		overrideKBenchPVCMount(job, pvc)
		return nil
	}

	job, pod, cleanup, err := k8s.EnsureJobCompleted(
		ctx,
		s.Clients,
		s.Name(),
		runID,
		namespace,
		"", // let k8s pick any node for this pod
		opts.KBenchImageName+":"+opts.KBenchImageTag,
		opts.JobActiveDeadline,
		opts.JobPodReadyTimeout,
		opts.JobPodTTLAfterFinished,
		opts.JobSuspend,
		overrides,
	)
	defer func() {
		if cleanup != nil {
			if err := cleanup(); err != nil {
				klog.V(3).ErrorS(err, "failed to clean up job", "suite", s.Name(), "case", caseName)
			}
		}
	}()
	if err != nil {
		return suites.NewCaseResultErrored(s.Name(), start, time.Now(), fmt.Errorf("job not completed: %v", err))
	}
	klog.V(3).InfoS("job is completed\n", "name", job.Name, "namespace", job.Namespace, "pod", pod.Name)

	// fetch the latest bound pvc to ensure we have the correct status and volume
	// name. this is unlikely to return an error since the job has completed.
	bound, err := k8s.WaitPVCBound(
		ctx,
		s.Clients,
		pvc,
		opts.PVCBoundTimeout,
	)
	if err != nil {
		return suites.NewCaseResultErrored(s.Name(), start, time.Now(), fmt.Errorf("pvc not bound: %v", err))
	}

	logOpts := &corev1.PodLogOptions{}
	result := &suites.PodLogResult{
		Subject: fmt.Sprintf("%s/%s", pod.GetNamespace(), pod.GetName()),
	}
	logs, err := s.K8sClientSet.CoreV1().Pods(namespace).GetLogs(pod.GetName(), logOpts).Stream(ctx)
	if err != nil {
		result.Err = fmt.Sprintf("failed to get logs for pod %s: %v", pod.GetName(), err)
	}
	if logs != nil {
		//nolint:errcheck
		defer logs.Close()

		b, err := io.ReadAll(logs)
		if err != nil {
			result.Err = fmt.Sprintf("failed to read logs for pod %s: %v", pod.GetName(), err)
		} else {
			result.Logs = string(b)
		}
	}

	caseResult := suites.NewCaseResult(caseName, start, time.Now(), nil, nil, job, pod, bound)
	caseResult = caseResult.WithPodLogResults([]*suites.PodLogResult{result})
	return caseResult
}

func (s *KBenchSuite) monitoring(
	ctx context.Context,
	caseName string,
	namespace string,
	nodeName string,
	pvcName string,
	kbenchRunDuration time.Duration,
	opts *KBenchOptions,
) *suites.CaseResult {
	start := time.Now()
	ready, err := k8s.MonitoringEnabled(ctx, s.Clients, opts.MonitoringNamespace, opts.MonitoringAddonName)
	if err != nil {
		err = fmt.Errorf("failed to check if monitoring addon is enabled, skipping promql execution: %w", err)
		return suites.NewCaseResultSkipped(caseName, start, time.Now(), err)
	}

	// skip if not ready
	if !ready {
		err = fmt.Errorf("monitoring add-on is not ready, skipping promql execution")
		return suites.NewCaseResultSkipped(caseName, start, time.Now(), err)
	}

	results := s.execPromQL(ctx, namespace, nodeName, pvcName, kbenchRunDuration, opts)
	caseResult := suites.NewCaseResult(caseName, start, time.Now(), nil, results)
	return caseResult
}

func (s *KBenchSuite) execPromQL(
	ctx context.Context,
	namespace string,
	nodeName string,
	pvcName string,
	kbenchRunDuration time.Duration,
	opts *KBenchOptions,
) []*suites.MetricResult {
	var (
		queryRangeWithRampTime = int(opts.RampTime.Seconds()) + int(kbenchRunDuration.Seconds())
		queryRange             = int(kbenchRunDuration.Seconds())
	)

	// validityCheckQueries are queries that check the validity of the test, such as
	// whether the volume remained. Any non-normal values imply that the test result
	// is invalid.
	// a result of 1 means healthy at every sample during the run; 0 means it was
	// not healthy at least once.
	// the inner min_over_time checks the time series over the query range, assessing
	// if the volume was ever unhealthy during the range window.
	// the outer min handles multiple time series for the same volume (e.g., due to
	// volume movement among nodes). It ensures only the lowest value is returned.
	validityCheckQueries := []string{
		// pvc should remain healthy during the test
		fmt.Sprintf(`min(min_over_time(longhorn_volume_robustness{namespace='%s', pvc='%s', pvc_namespace='%s', state='healthy'}[%ds]))`, opts.LonghornNamespace, pvcName, namespace, queryRangeWithRampTime),

		// volume should remain attached
		fmt.Sprintf(`min(min_over_time(longhorn_volume_state{namespace='%s', pvc='%s', pvc_namespace='%s', state='attached'}[%ds]))`, opts.LonghornNamespace, pvcName, namespace, queryRangeWithRampTime),
	}

	// overheadQueries are queries that check the resource usage of the instance
	// manager. these overheads happen outside of the kbench pods so they are not
	// captured in the kbench results, but they are important to understand the
	// overall performance of the system.
	overheadQueries := []string{
		fmt.Sprintf(`sum(avg_over_time(longhorn_instance_manager_cpu_usage_millicpu{namespace='%s'}[%ds])) by (node)`, opts.LonghornNamespace, queryRange),
		fmt.Sprintf(`sum(avg_over_time(longhorn_instance_manager_memory_usage_bytes{namespace='%s'}[%ds])) by (node)`, opts.LonghornNamespace, queryRange),
	}

	var metricResults []*suites.MetricResult
	for _, query := range validityCheckQueries {
		v, _, err := prom.RunInstant(ctx, s.PromClient, query)
		result := &suites.MetricResult{
			Query:   query,
			Samples: v,
		}
		if err != nil {
			result.Err = err.Error()
		} else if len(v) == 0 {
			result.Err = fmt.Sprintf("no samples returned for volume validity check query %q", query)
		}
		metricResults = append(metricResults, result)
	}

	for _, query := range overheadQueries {
		v, _, err := prom.RunInstant(ctx, s.PromClient, query)
		result := &suites.MetricResult{
			Query:   query,
			Samples: v,
		}
		if err != nil {
			result.Err = err.Error()
		}
		metricResults = append(metricResults, result)
	}
	return metricResults
}

func overrideKBenchSpec(caseName string, runID string, job *batchv1.Job, opts *KBenchOptions) {
	fileName := fmt.Sprintf("/volume/hvperf-%s-%s.perf", caseName, runID)
	job.Spec.Template.Spec.HostNetwork = false
	job.Spec.Template.Spec.Containers[0].Command = []string{} // use kbench default entrypoint
	job.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
		{Name: "MODE", Value: string(opts.Mode)},
		{Name: "FILE_NAME", Value: fileName},
		{Name: "SIZE", Value: opts.TestSize},
		{Name: "CPU_IDLE_PROF", Value: string(opts.CPUIdleProf)},
		{Name: "IOPS_BS", Value: opts.IOPSBlockSize},
		{Name: "IOPS_IODEPTH", Value: strconv.Itoa(opts.IOPSIoDepth)},
		{Name: "IOPS_NUMJOBS", Value: strconv.Itoa(opts.IOPSNumJobs)},
		{Name: "BW_BS", Value: opts.BandwidthBlockSize},
		{Name: "BW_IODEPTH", Value: strconv.Itoa(opts.BandwidthIoDepth)},
		{Name: "BW_NUMJOBS", Value: strconv.Itoa(opts.BandwidthNumJobs)},
		{Name: "LAT_BS", Value: opts.LatencyBlockSize},
		{Name: "LAT_IODEPTH", Value: strconv.Itoa(opts.LatencyIoDepth)},
		{Name: "LAT_NUMJOBS", Value: strconv.Itoa(opts.LatencyNumJobs)},
	}
	job.Spec.Template.Spec.Containers[0].Resources = corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    opts.KBenchLimitCPU,
			corev1.ResourceMemory: opts.KBenchLimitMem,
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    opts.KBenchRequestCPU,
			corev1.ResourceMemory: opts.KBenchRequestMem,
		},
	}
}

func overrideKBenchPVCMount(job *batchv1.Job, pvc *corev1.PersistentVolumeClaim) {
	job.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
		{Name: "vol", MountPath: "/volume/"},
		{Name: "temp-storage", MountPath: "/temp"},
	}
	job.Spec.Template.Spec.Volumes = []corev1.Volume{
		{
			Name: "vol",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvc.GetName(),
				},
			},
		},
		{
			Name: "temp-storage",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}
}

func (s *KBenchSuite) SetClients(clients *suites.Clients) {
	s.Clients = clients
}

func (s *KBenchSuite) SetProgressReporter(reporter *suites.ProgressReporter) {
	s.ProgressReporter = reporter
}

type KBenchOptions struct {
	CPUIdleProf        CPUIdleProfType
	BandwidthBlockSize string
	BandwidthIoDepth   int
	BandwidthNumJobs   int
	IOPSBlockSize      string
	IOPSIoDepth        int
	IOPSNumJobs        int
	LatencyBlockSize   string
	LatencyIoDepth     int
	LatencyNumJobs     int
	Mode               ModeType
	RampTime           time.Duration // used in the promql range; kbench doesn't support overriding ramp time
	TestSize           string

	JobActiveDeadline      time.Duration
	JobPodTTLAfterFinished time.Duration
	JobPodReadyTimeout     time.Duration
	JobSuspend             bool

	KBenchImageName  string
	KBenchImageTag   string
	KBenchLimitCPU   resource.Quantity
	KBenchLimitMem   resource.Quantity
	KBenchRequestCPU resource.Quantity
	KBenchRequestMem resource.Quantity

	LonghornNamespace     string
	MonitoringAddonName   string
	MonitoringNamespace   string
	NamespaceReadyTimeout time.Duration

	PVCBoundTimeout     time.Duration
	PVCStorageClassName string
	PVCStorageSize      resource.Quantity
}

// NewKBenchOptions returns the options for the kbench test suite. It merges the
// default global options with the default kbench options, giving precedence to
// any override options.
func NewKBenchOptions(override suites.Options) (*KBenchOptions, error) {
	merged := options.DefaultGlobalOptions().
		Merge(options.DefaultKBenchOptions()).
		Merge(override)
	o, err := options.FromOptions[*KBenchOptions](merged)
	if err != nil {
		return nil, err
	}

	q, err := resource.ParseQuantity(o.TestSize)
	if err != nil {
		return nil, err
	}
	if q.Cmp(o.PVCStorageSize) >= 0 {
		return nil, fmt.Errorf("test size %s must be smaller than PVC storage size %s", o.TestSize, o.PVCStorageSize.String())
	}

	return o, nil
}

type CPUIdleProfType string

const (
	CPUIdleProfDisabled CPUIdleProfType = "disabled"
	CPUIdleProfEnabled  CPUIdleProfType = "enabled"
)

type ModeType string

const (
	ModeFull              ModeType = "full"
	ModeRandReadIOPS      ModeType = "random-read-iops"
	ModeRandReadLatency   ModeType = "random-read-latency"
	ModeRandWriteIOPS     ModeType = "random-write-iops"
	ModeRandWriteLatency  ModeType = "random-write-latency"
	ModeSeqReadBandwidth  ModeType = "sequential-read-bandwidth"
	ModeSeqWriteBandwidth ModeType = "sequential-write-bandwidth"
)
