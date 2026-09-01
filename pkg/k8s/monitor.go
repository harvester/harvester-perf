package k8s

import (
	"context"
	"time"

	"github.com/harvester/hvperf/pkg/suites"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1apply "k8s.io/client-go/applyconfigurations/meta/v1"

	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monv1apply "github.com/prometheus-operator/prometheus-operator/pkg/client/applyconfiguration/monitoring/v1"
)

// EnsurePodMonitor ensures that the specified PodMonitor exists.
func EnsurePodMonitor(
	ctx context.Context,
	clients *suites.Clients,
	name string,
	namespace string,
	readyTimeout time.Duration,
) (*monv1.PodMonitor, error) {
	var (
		applyConfig       = monv1apply.PodMonitor(name, namespace)
		namespaceSelector = monv1apply.NamespaceSelector().WithMatchNames(namespace)
		selector          = metav1apply.LabelSelector().WithMatchLabels(map[string]string{
			"component": "etcd",
			"tier":      "control-plane",
		})
		metricsEndpoints = monv1apply.PodMetricsEndpoint().WithPort("metrics").WithPath("/metrics").WithInterval("30s").WithScheme("http").WithScrapeTimeout("10s")
	)
	applyConfigSpec := monv1apply.PodMonitorSpec().
		WithNamespaceSelector(namespaceSelector).
		WithSelector(selector).
		WithPodMetricsEndpoints(metricsEndpoints)

	applyConfig = applyConfig.WithSpec(applyConfigSpec)
	return clients.MonClientSet.MonitoringV1().PodMonitors(namespace).Apply(ctx, applyConfig, metav1.ApplyOptions{
		FieldManager: DefaultSSAFieldManager,
	})
}

// DeletePodMonitor deletes the specified PodMonitor.
func DeletePodMonitor(ctx context.Context, clients *suites.Clients, name string, namespace string) error {
	return clients.MonClientSet.MonitoringV1().PodMonitors(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}
