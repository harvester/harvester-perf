package k8s

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	LabelHarvesterAppName   = "app.kubernetes.io/name"
	LabelHarvesterAppValue  = "hvperf"
	LabelHarvesterSuiteName = "harvesterhci.io/suite-name"
	LabelHarvesterRunID     = "harvesterhci.io/run-id"
)

func defaultLabelSelector(suiteName, runID string) *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			LabelHarvesterAppName:   LabelHarvesterAppValue,
			LabelHarvesterSuiteName: suiteName,
			LabelHarvesterRunID:     runID,
		},
	}
}
