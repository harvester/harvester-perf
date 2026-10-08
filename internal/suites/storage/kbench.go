package storage

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/harvester/hvperf/internal/suites/options"
	"github.com/harvester/hvperf/pkg/k8s"
	"github.com/harvester/hvperf/pkg/suites"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
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
	o, err := KBenchOptionsDefaults()
	if err != nil {
		return suites.SuiteResult{
			Name:  s.Name(),
			RunID: runID,
			Err:   err.Error(),
		}
	}

	if o.TestSize.Cmp(o.PVCStorageSize) >= 0 {
		return suites.SuiteResult{
			Name:  s.Name(),
			RunID: runID,
			Err:   fmt.Sprintf("test size %s must be smaller than PVC storage size %s", o.TestSize.String(), o.PVCStorageSize.String()),
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
	caseName := "kbench-pvc"
	s.CaseStart(s.Name(), caseName)
	caseResult := s.kbenchPodPVC(ctx, caseName, runID, namespace, o)
	caseResults = append(caseResults, caseResult)
	s.CaseDone(s.Name(), caseName, caseResult.State, time.Since(caseResult.DateTimeStart))

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

func (s *KBenchSuite) kbenchPodPVC(ctx context.Context, caseName string, runID string, namespace string, opts *KBenchOptions) *suites.CaseResult {
	start := time.Now()
	pvc, cleanup, err := k8s.EnsurePVCReady(
		ctx,
		s.Clients,
		s.Name(),
		runID,
		namespace,
		opts.PVCReadyTimeout,
		opts.PVCStorageClassName,
		opts.PVCStorageSize,
	)
	defer func() {
		if cleanup != nil {
			// nolint: errcheck
			cleanup()
		}
	}()
	if err != nil {
		return suites.NewCaseResultErrored(s.Name(), time.Now(), time.Now(), fmt.Errorf("pvc not ready: %v", err))
	}
	klog.V(3).InfoS("pvc is ready\n", "name", pvc.Name, "namespace", pvc.Namespace)

	// job spec from https://github.com/longhorn/kbench/blob/main/deploy/fio.yaml
	overrides := func(job *batchv1.Job) error {
		job.Spec.Template.Spec.HostNetwork = false
		job.Spec.Template.Spec.Containers[0].Command = []string{} // use kbench default entrypoint
		job.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
			{Name: "MODE", Value: string(opts.Mode)},
			{Name: "FILE_NAME", Value: "/volume/test"},
			{Name: "SIZE", Value: opts.TestSize.String()},
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
		job.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
			{Name: "vol", MountPath: "/volume/"},
			{Name: "temp-storage", MountPath: "/temp"},
		}
		job.Spec.Template.Spec.Volumes = []corev1.Volume{
			{
				Name: "vol",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvc.Name,
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
		return nil
	}
	job, pod, err := k8s.EnsureJobCompleted(
		ctx,
		s.Clients,
		s.Name(),
		runID,
		namespace,
		"", // let k8s pick any node for this pod
		opts.JobPodImageName+":"+opts.JobPodImageTag,
		opts.JobActiveDeadline,
		opts.JobPodReadyTimeout,
		opts.JobPodTTLAfterFinished,
		opts.JobSuspend,
		overrides,
	)
	if err != nil {
		return suites.NewCaseResultErrored(s.Name(), time.Now(), time.Now(), fmt.Errorf("job not ready: %v", err))
	}
	klog.V(3).InfoS("job is ready\n", "name", job.Name, "namespace", job.Namespace, "pod", pod.Name)

	logOpts := &corev1.PodLogOptions{}
	result := &suites.PodLogResult{
		Subject: fmt.Sprintf("%s/%s", pod.GetNamespace(), pod.GetName()),
	}
	logs, err := s.K8sClientSet.CoreV1().Pods(namespace).GetLogs(pod.GetName(), logOpts).Stream(ctx)
	if err != nil {
		result.Err = fmt.Sprintf("failed to get logs for pod %s: %v", pod.GetName(), err)
	}
	if logs != nil {
		b, err := io.ReadAll(logs)
		if err != nil {
			result.Err = fmt.Sprintf("failed to read logs for pod %s: %v", pod.GetName(), err)
		} else {
			result.Logs = string(b)
		}
	}

	caseResult := suites.NewCaseResult(caseName, start, time.Now(), nil, nil)
	caseResult = caseResult.WithPodLogResults([]*suites.PodLogResult{result})
	return caseResult
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
	TestSize           resource.Quantity

	JobActiveDeadline      time.Duration
	JobPodContainerName    string
	JobPodImageName        string
	JobPodImageTag         string
	JobPodTTLAfterFinished time.Duration
	JobPodReadyTimeout     time.Duration
	JobSuspend             bool

	NamespaceReadyTimeout time.Duration

	PVCReadyTimeout     time.Duration
	PVCStorageClassName string
	PVCStorageSize      resource.Quantity
}

// KBenchOptionsDefaults returns the default options for the kbench test suite.
// It merges the default global options with the default kbench options, giving
// precedence to the global options for job-related settings.
func KBenchOptionsDefaults() (*KBenchOptions, error) {
	sysOpts, err := options.FromOptions[*KBenchOptions](options.DefaultGlobalOptions())
	if err != nil {
		return nil, err
	}

	o, err := options.FromOptions[*KBenchOptions](options.DefaultKBenchOptions())
	if err != nil {
		return nil, err
	}
	o.JobActiveDeadline = sysOpts.JobActiveDeadline
	o.JobPodContainerName = sysOpts.JobPodContainerName
	o.JobPodTTLAfterFinished = sysOpts.JobPodTTLAfterFinished
	o.JobPodReadyTimeout = sysOpts.JobPodReadyTimeout
	o.JobSuspend = sysOpts.JobSuspend
	o.NamespaceReadyTimeout = sysOpts.NamespaceReadyTimeout
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
	ModeQuick             ModeType = "quick"
	ModeRandReadIOPS      ModeType = "random-read-iops"
	ModeRandReadLatency   ModeType = "random-read-latency"
	ModeRandWriteIOPS     ModeType = "random-write-iops"
	ModeRandWriteLatency  ModeType = "random-write-latency"
	ModeSeqReadBandwidth  ModeType = "sequential-read-bandwidth"
	ModeSeqWriteBandwidth ModeType = "sequential-write-bandwidth"
)
