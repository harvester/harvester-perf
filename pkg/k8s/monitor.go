package k8s

import (
	"context"
	"fmt"

	"github.com/harvester/hvperf/pkg/suites"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	metav1apply "k8s.io/client-go/applyconfigurations/meta/v1"

	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monv1apply "github.com/prometheus-operator/prometheus-operator/pkg/client/applyconfiguration/monitoring/v1"
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

// EnsurePodMonitor ensures that the specified PodMonitor exists.
func EnsurePodMonitor(
	ctx context.Context,
	clients *suites.Clients,
	name string,
	namespace string,
	targetNamespace string,
) (*monv1.PodMonitor, error) {
	var (
		applyConfig       = monv1apply.PodMonitor(name, namespace)
		namespaceSelector = monv1apply.NamespaceSelector().WithMatchNames(targetNamespace)
		selector          = metav1apply.LabelSelector().WithMatchLabels(map[string]string{
			"component": "etcd",
			"tier":      "control-plane",
		})
		metricsEndpoints = monv1apply.PodMetricsEndpoint().
					WithPort("metrics").
					WithPath("/metrics").
					WithScheme("http").
					WithScrapeTimeout("10s")
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
