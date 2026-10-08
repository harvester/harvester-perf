package options

import (
	"reflect"
	"testing"
	"time"

	"github.com/harvester/hvperf/pkg/suites"
)

func TestDefaultGlobalOptions(t *testing.T) {
	want := &suites.Options{
		"EtcdNamespace":                   "kube-system",
		"EtcdReadyTimeout":                300 * time.Second,
		"JobActiveDeadline":               3600 * time.Second,
		"JobPodContainerName":             "benchmark",
		"JobPodImageName":                 "registry.suse.com/bci/bci-base",
		"JobPodImageTag":                  "latest",
		"JobPodTTLAfterFinished":          300 * time.Second,
		"JobPodReadyTimeout":              3600 * time.Second,
		"JobSuspend":                      false,
		"MonitoringAddonName":             "rancher-monitoring",
		"MonitoringNamespace":             "cattle-monitoring-system",
		"MonitoringRangeDuration":         300 * time.Second,
		"MonitoringWaitPodMonitorTimeout": 600 * time.Second,
		"NamespaceReadyTimeout":           300 * time.Second,
	}

	if got := DefaultGlobalOptions(); !reflect.DeepEqual(got, want) {
		t.Errorf("DefaultGlobalOptions() = %+v, want %+v", got, want)
	}
}
