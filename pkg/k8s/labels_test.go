package k8s

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDefaultLabelSelector(t *testing.T) {
	got := defaultLabelSelector("node-capacity", "abc123")
	want := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			LabelHarvesterAppName:   LabelHarvesterAppValue,
			LabelHarvesterSuiteName: "node-capacity",
			LabelHarvesterRunID:     "abc123",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("defaultLabelSelector() = %+v, want %+v", got, want)
	}
}
