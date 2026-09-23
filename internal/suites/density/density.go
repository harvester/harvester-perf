package density

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"go.yaml.in/yaml/v4"
	"golang.org/x/sync/errgroup"

	pkgprom "github.com/harvester/hvperf/pkg/prometheus"
	"github.com/harvester/hvperf/pkg/resource"
	pkgsuites "github.com/harvester/hvperf/pkg/suites"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/retry"
)

// systemNamespaceFilter scopes CPU/memory queries to system namespaces.
const (
	systemNamespaceFilter = `namespace=~"kube-system|kubevirt|cdi|longhorn-system|cattle-monitoring-system|cattle-logging-system|harvester-system|fleet-local"`
	promWindowMin         = 5 * time.Minute
)

// promWindow returns a Prometheus range string clamped to promWindowMin.
// The Prometheus query range must be at least 5m to provide more stable metrics under load.
func promWindow(elapsed time.Duration) string {
	if elapsed < promWindowMin {
		elapsed = promWindowMin
	}
	return fmt.Sprintf("%ds", int(elapsed.Seconds()))
}

// dynamicQueries returns 17 PromQL queries scoped to the run window w.
func dynamicQueries(w string) []string {
	return []string{
		// Namespace CPU/memory during the run.
		fmt.Sprintf(`sum by (namespace, node) (rate(container_cpu_usage_seconds_total{container!="", pod!="", `+systemNamespaceFilter+`}[%s]))`, w),
		fmt.Sprintf(`avg_over_time((sum by (namespace, node) (container_memory_working_set_bytes{container!="", pod!="", `+systemNamespaceFilter+`}))[%s:30s])`, w),
		// virt-api component stats over the run.
		fmt.Sprintf(`avg_over_time((sum(rate(container_cpu_usage_seconds_total{namespace="harvester-system",pod=~"virt-api-.*",container!=""}[2m])))[%s:30s])`, w),
		fmt.Sprintf(`avg_over_time((sum(container_memory_working_set_bytes{namespace="harvester-system",pod=~"virt-api-.*",container!=""}))[%s:30s])`, w),
		fmt.Sprintf(`max_over_time((sum(container_memory_working_set_bytes{namespace="harvester-system",pod=~"virt-api-.*",container!=""}))[%s:30s])`, w),
		fmt.Sprintf(`min_over_time((sum(container_memory_working_set_bytes{namespace="harvester-system",pod=~"virt-api-.*",container!=""}))[%s:30s])`, w),
		// virt-controller component stats over the run.
		fmt.Sprintf(`avg_over_time((sum(rate(container_cpu_usage_seconds_total{namespace="harvester-system",pod=~"virt-controller-.*",container!=""}[2m])))[%s:30s])`, w),
		fmt.Sprintf(`avg_over_time((sum(container_memory_working_set_bytes{namespace="harvester-system",pod=~"virt-controller-.*",container!=""}))[%s:30s])`, w),
		fmt.Sprintf(`max_over_time((sum(container_memory_working_set_bytes{namespace="harvester-system",pod=~"virt-controller-.*",container!=""}))[%s:30s])`, w),
		fmt.Sprintf(`min_over_time((sum(container_memory_working_set_bytes{namespace="harvester-system",pod=~"virt-controller-.*",container!=""}))[%s:30s])`, w),
		// virt-handler component stats over the run.
		fmt.Sprintf(`avg_over_time((sum(rate(container_cpu_usage_seconds_total{namespace="harvester-system",pod=~"virt-handler-.*",container!=""}[2m])))[%s:30s])`, w),
		fmt.Sprintf(`avg_over_time((sum(container_memory_working_set_bytes{namespace="harvester-system",pod=~"virt-handler-.*",container!=""}))[%s:30s])`, w),
		fmt.Sprintf(`max_over_time((sum(container_memory_working_set_bytes{namespace="harvester-system",pod=~"virt-handler-.*",container!=""}))[%s:30s])`, w),
		fmt.Sprintf(`min_over_time((sum(container_memory_working_set_bytes{namespace="harvester-system",pod=~"virt-handler-.*",container!=""}))[%s:30s])`, w),
		// VM creation latency — only VMs that reached Running contribute.
		fmt.Sprintf(`histogram_quantile(0.50, sum by (le) (rate(kubevirt_vmi_phase_transition_time_from_creation_seconds_bucket{phase="Running"}[%s])))`, w),
		fmt.Sprintf(`histogram_quantile(0.95, sum by (le) (rate(kubevirt_vmi_phase_transition_time_from_creation_seconds_bucket{phase="Running"}[%s])))`, w),
		fmt.Sprintf(`histogram_quantile(0.99, sum by (le) (rate(kubevirt_vmi_phase_transition_time_from_creation_seconds_bucket{phase="Running"}[%s])))`, w),
	}
}

var _ pkgsuites.Suite = &DensitySuite{}

func init() {
	suite := &DensitySuite{}
	suite.Marshal = suite
	pkgsuites.Register(suite)
}

type DensitySuite struct {
	pkgsuites.SuiteMarshaler
	*pkgsuites.Clients
}

// densityOptions is the whole density block of hvperf.yaml: the ramp knobs,
// plus the VM and VMImage shape nested underneath.
type densityOptions struct {
	Concurrency  int              `yaml:"concurrency"`
	WaitTimeout  time.Duration    `yaml:"waitTimeout"`
	PerVMTimeout time.Duration    `yaml:"perVMTimeout"`
	VMImage      resource.VMImage `yaml:"vmImage"`
	VM           resource.VM      `yaml:"vm"`
}

func defaultOptions() densityOptions {
	return densityOptions{
		Concurrency:  10,
		WaitTimeout:  30 * time.Minute,
		PerVMTimeout: 5 * time.Minute,
		VMImage: resource.VMImage{
			URL: "https://download.cirros-cloud.net/0.6.2/cirros-0.6.2-x86_64-disk.img",
		},
		VM: resource.VM{
			StorageClass: "longhorn",
			DiskSize:     "1Gi",
			Memory:       "90Mi",
			CPU:          "100m",
		},
	}
}

// loadOptions reads the density block from path (e.g. hvperf.yaml) over the
// defaults. Missing file → defaults. Parse error → error.
func loadOptions(path string) (densityOptions, error) {
	o := defaultOptions()
	data, err := os.ReadFile(path)
	if err != nil {
		slog.Info("config not found, using defaults", "path", path)
		return o, nil
	}
	slog.Info("loaded config", "path", path)
	cfg := struct {
		Density *densityOptions `yaml:"density"`
	}{Density: &o}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return densityOptions{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return o, nil
}

func (s *DensitySuite) SetProgressReporter(_ *pkgsuites.ProgressReporter) {}
func (s *DensitySuite) Name() string                                      { return "density" }
func (s *DensitySuite) Description() string {
	return "import a VM image and create a Harvester VM"
}
func (s *DensitySuite) IsReadWrite() bool                     { return true }
func (s *DensitySuite) SetClients(clients *pkgsuites.Clients) { s.Clients = clients }

func (s *DensitySuite) RunE(ctx context.Context, runID, namespace string, opts pkgsuites.Options) pkgsuites.SuiteResult {
	runStart := time.Now()
	configPath := "./hvperf.yaml"
	if p, ok := opts["configFile"].(string); ok && p != "" {
		configPath = p
	}
	result := pkgsuites.SuiteResult{Name: s.Name(), RunID: runID}
	o, err := loadOptions(configPath)
	if err != nil {
		result.Err = err.Error()
		return result
	}
	defer s.cleanup(namespace, runID)

	image := o.VMImage
	image.Namespace, image.Name, image.RunID = namespace, runID+"-image", runID
	imageResult, imageErr := s.runImageImport(ctx, image, o.WaitTimeout)
	result.Results = append(result.Results, imageResult)
	if imageErr != nil {
		return result
	}

	result.Results = append(result.Results, s.createVMsUntilFailure(ctx, o, namespace, runID, image.Namespace+"/"+image.Name, runStart))
	metricResult, metricsErr := s.collectDensityMetrics(ctx, runStart)
	result.Results = append(result.Results, metricResult)
	if metricsErr != nil {
		result.Err = metricsErr.Error()
	}
	return result
}

func (s *DensitySuite) runImageImport(ctx context.Context, image resource.VMImage, waitTimeout time.Duration) (*pkgsuites.CaseResult, error) {
	start := time.Now()
	err := resource.CreateAndWaitVMImage(ctx, s.DynClientSet, image, waitTimeout)
	return caseResult("vm-image-import", start, err), err
}

func (s *DensitySuite) createVMsUntilFailure(ctx context.Context, o densityOptions, namespace, runID, imageID string, runStart time.Time) *pkgsuites.CaseResult {
	start := time.Now()
	vmErr := s.createAndWaitVMs(ctx, o, namespace, runID, imageID)

	ready, countErr := s.countReadyVMs(ctx, namespace, runID)
	stoppedBy := "completed"
	switch {
	case errors.Is(vmErr, context.DeadlineExceeded) && ctx.Err() != nil:
		stoppedBy = "run deadline"
	case errors.Is(vmErr, context.DeadlineExceeded):
		stoppedBy = "per-VM deadline"
	case errors.Is(vmErr, context.Canceled):
		stoppedBy = "cancelled"
	case vmErr != nil:
		stoppedBy = "VM creation error"
	}
	return caseResult(fmt.Sprintf("vm-create: %d ready, stopped by %s", ready, stoppedBy), start, errors.Join(vmErr, countErr))
}

func (s *DensitySuite) collectDensityMetrics(ctx context.Context, runStart time.Time) (*pkgsuites.CaseResult, error) {
	start := time.Now()
	metrics, err := s.measure(ctx, dynamicQueries(promWindow(time.Since(runStart)))...)
	result := pkgsuites.NewCaseResult("density metrics", start, time.Now(), nil, metrics)
	if err != nil {
		result.Err = err.Error()
		result.FinalizeState()
	}
	return result, err
}

// createAndWaitVMs ramps VM creation until one VM fails to come up. That is the
// ceiling, and the reason does not change the decision: the scheduler refusing it
// for lack of resources and it never becoming ready in time both mean the cluster
// has stopped accepting VMs. Stopping on the first failure is also what keeps an
// uncapped ramp finite.
//
// VMs already created are left in place — countReadyVMs is what produces the
// number, so nothing is lost by stopping here.
func (s *DensitySuite) createAndWaitVMs(ctx context.Context, o densityOptions, namespace, runID, imageID string) error {
	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(o.Concurrency)

	for index := 0; ctx.Err() == nil; index++ {
		vm := o.VM
		vm.Namespace, vm.RunID, vm.ImageID = namespace, runID, imageID
		vm.Name = fmt.Sprintf("%s-vm-%d", runID, index)

		eg.Go(func() error {
			vmCtx, cancel := context.WithTimeout(ctx, o.PerVMTimeout)
			defer cancel()

			err := resource.CreateAndWaitVM(vmCtx, s.DynClientSet, vm)
			if err == nil {
				return nil
			}
			if errors.Is(err, context.DeadlineExceeded) {
				if reason := s.unschedulableReason(ctx, vm); reason != "" {
					err = fmt.Errorf("%s: %w", reason, err)
				}
			}
			return fmt.Errorf("stopped at VM %s: %w", vm.Name, err)
		})
	}
	return eg.Wait()
}

// countReadyVMs counts run-labelled VMs reporting status.ready — the density
// ceiling. Measured once after the ramp settles rather than per VM, so a VM that
// outlived its watch still counts.
func (s *DensitySuite) countReadyVMs(ctx context.Context, namespace, runID string) (int, error) {
	list, err := s.DynClientSet.Resource(resource.VMGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: resource.RunLabel + "=" + runID,
	})
	if err != nil {
		return 0, fmt.Errorf("count ready VMs: %w", err)
	}
	var ready int
	for _, item := range list.Items {
		if ok, _, _ := unstructured.NestedBool(item.Object, "status", "ready"); ok {
			ready++
		}
	}
	return ready, nil
}

func (s *DensitySuite) unschedulableReason(ctx context.Context, vm resource.VM) string {
	selector := "vm.kubevirt.io/name=" + vm.Name
	pods, err := s.K8sClientSet.CoreV1().Pods(vm.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return ""
	}
	for _, pod := range pods.Items {
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse {
				return condition.Message
			}
		}
	}
	return ""
}

func (s *DensitySuite) measure(ctx context.Context, queries ...string) ([]*pkgsuites.MetricResult, error) {
	metrics := make([]*pkgsuites.MetricResult, 0, len(queries))
	for _, query := range queries {
		samples, _, err := pkgprom.RunInstant(ctx, s.PromClient, query)
		if err != nil {
			return metrics, err
		}
		metrics = append(metrics, &pkgsuites.MetricResult{Query: query, Samples: samples})
	}
	return metrics, nil
}

func (s *DensitySuite) cleanup(namespace, runID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
	defer cancel()
	selector := metav1.ListOptions{LabelSelector: resource.RunLabel + "=" + runID}

	if err := retry.OnError(retry.DefaultRetry, isRetryableError, func() error {
		return s.DynClientSet.Resource(resource.VMGVR).Namespace(namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, selector)
	}); err != nil {
		slog.Error("cleanup: delete VMs failed", "runID", runID, "err", err)
	}

	if err := resource.WaitForDeletion(ctx, s.DynClientSet, resource.VMGVR, namespace, selector); err != nil {
		slog.Error("cleanup: wait VMs failed", "runID", runID, "err", err)
	}

	pvNames, err := resource.DeleteRunVolumes(ctx, s.DynClientSet, namespace, runID)
	if err != nil {
		slog.Error("cleanup: list run PVCs failed", "runID", runID, "err", err)
		return
	}
	if err := resource.WaitLonghornVolumesByName(ctx, s.DynClientSet, pvNames); err != nil {
		slog.Error("cleanup: wait Longhorn volumes failed", "runID", runID, "err", err)
		return
	}

	if err := s.DynClientSet.Resource(resource.VMImageGVR).Namespace(namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, selector); err != nil {
		slog.Error("cleanup: delete VMImage failed", "runID", runID, "err", err)
	}
}

func isRetryableError(err error) bool {
	return apierrors.IsServerTimeout(err) ||
		apierrors.IsTooManyRequests(err) ||
		apierrors.IsServiceUnavailable(err)
}

func caseResult(name string, start time.Time, err error) *pkgsuites.CaseResult {
	return pkgsuites.NewCaseResult(
		name,
		start,
		time.Now(),
		[]*pkgsuites.CmdResult{{Cmd: name + " create", Err: errString(err)}},
		nil,
	)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
