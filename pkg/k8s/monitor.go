package k8s

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/harvester/hvperf/pkg/suites"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	metav1apply "k8s.io/client-go/applyconfigurations/meta/v1"

	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monv1apply "github.com/prometheus-operator/prometheus-operator/pkg/client/applyconfiguration/monitoring/v1"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

var addonGVR = schema.GroupVersionResource{
	Group:    "harvesterhci.io",
	Version:  "v1beta1",
	Resource: "addons",
}

// MonitoringEnabled checks if the rancher-monitoring addon is enabled in the
// specified namespace.
func MonitoringEnabled(ctx context.Context, c *suites.Clients, namespace, name string) (bool, error) {
	addon, err := c.DynClientSet.
		Resource(addonGVR).
		Namespace(namespace).
		Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("getting rancher-monitoring addon: %w", err)
	}

	enabled, found, err := unstructured.NestedBool(addon.Object, "spec", "enabled")
	if err != nil {
		return false, fmt.Errorf("monitoring addon reading spec.enabled: found=%v: %w", found, err)
	}
	if !found {
		return false, nil
	}
	return enabled, nil
}

// EnsurePodMonitor ensures that the specified PodMonitor exists. It returns a
// cleanup function to delete the PodMonitor and an error if any occurred during
// the process. Caller is responsible for calling the cleanup function to prevent
// continuous polling of the etcd pods.
//
// The function waits until Prometheus has successfully scraped
// metrics from all etcd pods before returning.
func EnsurePodMonitor(
	ctx context.Context,
	clients *suites.Clients,
	opts *PodMonitorOption,
	jobPod *corev1.Pod,
) (func() error, error) {
	if opts.EtcdCount <= 0 {
		return func() error {
			return nil
		}, fmt.Errorf("etcd count must be greater than 0, got %d", opts.EtcdCount)
	}

	// cleanup is the cleanup function to be returned to caller to delete
	// the PodMonitor created by EnsurePodMonitor. If left behind, Prometheus would
	// continue to poll the etcd pods.
	cleanup := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()
		return clients.MonClientSet.MonitoringV1().PodMonitors(opts.Namespace).Delete(ctx, opts.Name, metav1.DeleteOptions{})
	}

	var (
		applyConfig       = monv1apply.PodMonitor(opts.Name, opts.Namespace)
		namespaceSelector = monv1apply.NamespaceSelector().WithMatchNames(opts.TargetNamespace)
		selector          = metav1apply.LabelSelector().WithMatchLabels(opts.LabelSelector)
		metricsEndpoints  = monv1apply.PodMetricsEndpoint().
					WithPort(opts.MetricsPortName).
					WithPath(opts.MetricsPath).
					WithScheme(monv1.Scheme(opts.EndpointScheme)).
					WithScrapeTimeout("10s")
	)
	applyConfigSpec := monv1apply.PodMonitorSpec().
		WithNamespaceSelector(namespaceSelector).
		WithSelector(selector).
		WithPodMetricsEndpoints(metricsEndpoints)

	applyConfig = applyConfig.WithSpec(applyConfigSpec)
	if _, err := clients.MonClientSet.MonitoringV1().PodMonitors(opts.Namespace).Apply(ctx, applyConfig, metav1.ApplyOptions{
		FieldManager: DefaultSSAFieldManager,
	}); err != nil {
		return cleanup, err
	}
	applyTime := time.Now()

	var (
		// jobName is auto-assigned by the PodMonitor controller
		jobName = fmt.Sprintf("%s/%s", opts.Namespace, opts.Name)
		waitErr error
	)
	if err := wait.PollUntilContextTimeout(ctx, time.Second*30, opts.WaitTimeout, true, func(ctx context.Context) (bool, error) {
		// keep polling for the etcd job to be ready until timeout expired, ignoring
		// any errors to keep the wait alive.
		// intermediate errors are recorded using waitErr. waitErr gets reset on every
		// iteration so that when the wait times out, only the final state is returned
		// to the caller.
		waitErr = nil
		targets, err := clients.PromClient.Targets(ctx)
		if err != nil {
			waitErr = fmt.Errorf("failed to get prometheus targets... retrying: %w", err)
			return false, nil
		}

		var readyCount int
		for _, t := range targets.Active {
			if string(t.Labels["job"]) == jobName &&
				t.Health == promv1.HealthGood &&
				!t.LastScrape.IsZero() &&
				t.LastScrape.After(applyTime.Add(opts.RangeDuration)) {
				readyCount += 1
			}
		}

		return readyCount == opts.EtcdCount, nil
	}); err != nil {
		return cleanup, errors.Join(waitErr, err)
	}

	return cleanup, nil
}

type PodMonitorOption struct {
	Name            string
	Namespace       string
	EndpointScheme  string
	EtcdCount       int
	LabelSelector   map[string]string
	MetricsPortName string
	MetricsPath     string
	TargetNamespace string
	RangeDuration   time.Duration
	WaitTimeout     time.Duration
}
