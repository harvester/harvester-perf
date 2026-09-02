package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	"github.com/prometheus/common/model"
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
	metricsPortName string,
	metricsPath string,
	scheme string,
	targetNamespace string,
	monitoringServiceURL string,
	labelSelector map[string]string,
	jobPod *corev1.Pod,
	waitTimeout time.Duration,
) error {
	var (
		applyConfig       = monv1apply.PodMonitor(name, namespace)
		namespaceSelector = monv1apply.NamespaceSelector().WithMatchNames(targetNamespace)
		selector          = metav1apply.LabelSelector().WithMatchLabels(labelSelector)
		metricsEndpoints  = monv1apply.PodMetricsEndpoint().
					WithPort(metricsPortName).
					WithPath(metricsPath).
					WithScheme(monv1.Scheme(scheme)).
					WithScrapeTimeout("10s")
	)
	applyConfigSpec := monv1apply.PodMonitorSpec().
		WithNamespaceSelector(namespaceSelector).
		WithSelector(selector).
		WithPodMetricsEndpoints(metricsEndpoints)

	applyConfig = applyConfig.WithSpec(applyConfigSpec)
	if _, err := clients.MonClientSet.MonitoringV1().PodMonitors(namespace).Apply(ctx, applyConfig, metav1.ApplyOptions{
		FieldManager: DefaultSSAFieldManager,
	}); err != nil {
		return err
	}

	jobName := fmt.Sprintf("%s/%s", namespace, name)
	cmd := [][]string{
		{
			// check if the prom job is ready. the job's default name is set to the
			// namespace and name of the pod monitor
			"promtool",
			"query",
			"instant",
			"-o",
			"json",
			monitoringServiceURL,
			fmt.Sprintf("up{job='%s'}", jobName),
		},
	}

	var waitErr error
	if err := wait.PollUntilContextTimeout(ctx, time.Second*30, waitTimeout, true, func(ctx context.Context) (done bool, err error) {
		// keep polling for the etcd job to be ready until timeout expired,
		// ignoring any errors
		var r io.Reader
		r, err = ExecPod(ctx, clients, jobPod, cmd)
		if err != nil {
			waitErr = err
			return false, nil
		}

		b, readErr := io.ReadAll(r)
		if readErr != nil {
			waitErr = err
			return false, nil
		}

		var samples model.Samples
		if err := json.Unmarshal(b, &samples); err != nil {
			waitErr = err
			return false, nil
		}

		return samples[0] != nil && samples[0].Value == 1, nil
	}); err != nil {
		return errors.Join(waitErr, err)
	}
	return nil
}
