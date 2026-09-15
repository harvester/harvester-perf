package k8s

import (
	"context"
	"fmt"
	"time"

	"github.com/harvester/hvperf/pkg/suites"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubectl/pkg/util/podutils"
)

var EtcdLabelSelector = &metav1.LabelSelector{
	MatchLabels: map[string]string{
		"component": "etcd",
		"tier":      "control-plane",
	},
}

// EnsureEtcdReady checks if the etcd pods in the specified namespace are ready.
// It polls the status of the pods at the specified interval until all pods are
// ready or the timeout is reached.
func EnsureEtcdReady(
	ctx context.Context,
	c *suites.Clients,
	namespace string,
	readyTimeout time.Duration,
) (*corev1.PodList, bool, error) {
	var etcd *corev1.PodList
	ctxWithTimeout, cancel := context.WithTimeout(ctx, readyTimeout)
	pollInterval := 5 * time.Second
	defer cancel()

	if err := wait.PollUntilContextCancel(ctxWithTimeout, pollInterval, true, func(ctx context.Context) (bool, error) {
		list, err := c.K8sClientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: metav1.FormatLabelSelector(EtcdLabelSelector),
		})
		if err != nil {
			return false, err
		}
		etcd = list

		if len(etcd.Items) == 0 {
			return false, fmt.Errorf("no etcd pods found in namespace %s", namespace)
		}
		for _, pod := range etcd.Items {
			if !podutils.IsPodReady(&pod) {
				return false, nil
			}
		}
		return true, nil
	}); err != nil {
		return nil, false, err
	}

	return etcd, true, nil
}
