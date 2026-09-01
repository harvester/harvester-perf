package k8s

import (
	"context"
	"fmt"

	"github.com/harvester/hvperf/pkg/suites"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var addonGVR = schema.GroupVersionResource{
	Group:    "harvesterhci.io",
	Version:  "v1beta1",
	Resource: "addons",
}

// MonitoringEnabled checks if the rancher-monitoring addon is enabled in the
// specified namespace.
func MonitoringEnabled(ctx context.Context, c *suites.Clients) (bool, error) {
	addon, err := c.DynClientSet.
		Resource(addonGVR).
		Namespace(MonitoringNamespace).
		Get(ctx, MonitoringAddonName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("getting rancher-monitoring addon: %w", err)
	}

	enabled, found, err := unstructured.NestedBool(addon.Object, "spec", "enabled")
	if err != nil || !found {
		return false, fmt.Errorf("reading spec.enabled: found=%v: %w", found, err)
	}
	return enabled, nil
}
