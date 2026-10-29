package etcd

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/harvester/hvperf/pkg/suites"
)

func TestNewBenchmarkOptions(t *testing.T) {
	toolsDir := "/tmp"
	expected := &BenchmarkOptions{
		EtcdBenchmarkLocalPath:          "/tmp/benchmark",
		EtcdctlLocalPath:                "/tmp/etcdctl",
		EtcdNamespace:                   "kube-system",
		EtcdctlOutputFormat:             "simple",
		EtcdEndpoints:                   "https://127.0.0.1:2379",
		EtcdMetricsPath:                 "/metrics",
		EtcdMetricsPortName:             "metrics",
		EtcdMetricsScheme:               "http",
		EtcdReadyTimeout:                300 * time.Second,
		EtcdRemoteCopyTargetDir:         "/usr/local/bin/",
		EtcdRemoteTLSCertDir:            "/host/rancher/rke2/server/tls/etcd",
		JobActiveDeadline:               3600 * time.Second,
		JobPodImageName:                 "registry.suse.com/bci/bci-base",
		JobPodImageTag:                  "latest",
		JobPodNode:                      "",
		JobPodTTLAfterFinished:          300 * time.Second,
		JobPodReadyTimeout:              3600 * time.Second,
		JobSuspend:                      false,
		MonitoringAddonName:             "rancher-monitoring",
		MonitoringNamespace:             "cattle-monitoring-system",
		MonitoringOutputFormat:          "promql",
		MonitoringRangeDuration:         300 * time.Second,
		MonitoringWaitPodMonitorTimeout: 600 * time.Second,
		NamespaceReadyTimeout:           300 * time.Second,
		CheckPerfLoadSize:               "s",
		PutLoadSize:                     10000,
		PutKeySize:                      8,
		PutValSize:                      256,
		GRPCClientCount:                 1,
		GRPCConnCount:                   1,
	}
	actual, err := NewBenchmarkOptions(suites.Options{
		"toolsDir": toolsDir,
	})
	if err != nil {
		t.Fatalf("NewBenchmarkOptions() returned an error: %v", err)
	}

	if diff := cmp.Diff(actual, expected); diff != "" {
		t.Errorf("NewBenchmarkOptions() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewBenchmarkOptionsOverride(t *testing.T) {
	if _, err := NewBenchmarkOptions(nil); err == nil {
		t.Fatalf("NewBenchmarkOptions() expected error for nil options, got nil")
	}

	want, err := NewBenchmarkOptions(suites.Options{
		"toolsDir": "/tmp",
	})
	if err != nil {
		t.Fatal(err)
	}
	want.PutLoadSize = 500 // suite default overridden
	want.EtcdEndpoints = "https://10.0.0.1:2379"
	want.JobPodImageTag = "v1" // global default overridden
	want.MonitoringRangeDuration = 60 * time.Second
	want.EtcdBenchmarkLocalPath = "/tmp/benchmark"
	want.EtcdctlLocalPath = "/tmp/etcdctl"

	got, err := NewBenchmarkOptions(suites.Options{
		"toolsDir":                "/tmp",
		"putLoadSize":             500,
		"etcdEndpoints":           "https://10.0.0.1:2379",
		"jobPodImageTag":          "v1",
		"monitoringRangeDuration": 60 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewBenchmarkOptions() error = %v, want nil", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("NewBenchmarkOptions() mismatch (-want +got):\n%s", diff)
	}
}
