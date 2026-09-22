package k8s

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	LABEL_HARVESTER_APP_NAME   = "app.kubernetes.io/name"
	LABEL_HARVESTER_APP_VALUE  = "hvperf"
	LABEL_HARVESTER_SUITE_NAME = "harvesterhci.io/suite-name"
	LABEL_HARVESTER_RUN_ID     = "harvesterhci.io/run-id"
)

func defaultLabelSelector(suiteName, runID string) *metav1.LabelSelector {
	selector := fmt.Sprintf("%s=%s,%s=%s,%s=%s",
		LABEL_HARVESTER_APP_NAME,
		LABEL_HARVESTER_APP_VALUE,
		LABEL_HARVESTER_SUITE_NAME,
		suiteName,
		LABEL_HARVESTER_RUN_ID,
		runID)
	labelSelector, err := metav1.ParseToLabelSelector(selector)
	if err != nil {
		return &metav1.LabelSelector{
			MatchLabels: map[string]string{
				LABEL_HARVESTER_APP_NAME:   LABEL_HARVESTER_APP_VALUE,
				LABEL_HARVESTER_SUITE_NAME: suiteName,
				LABEL_HARVESTER_RUN_ID:     runID,
			},
		}
	}
	return labelSelector
}
