package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/harvester/hvperf/pkg/suites"
)

// fakeSuite embeds the Suite interface (nil) so only the methods canonicalizeRunOptions
// calls need implementing. any other call panics, which is what we want in a test.
type fakeSuite struct {
	suites.Suite
	SuiteName          string
	SuiteOptionsSchema suites.Options
}

func (s *fakeSuite) Name() string                  { return s.SuiteName }
func (s *fakeSuite) OptionsSchema() suites.Options { return s.SuiteOptionsSchema }

func TestCanonicalizeRunOptions(t *testing.T) {
	// the point of this test is to ensure that the canonicalization
	// process correctly transforms user-provided options into the expected canonical
	// form.
	fakeSuites := []suites.Suite{
		&fakeSuite{
			SuiteName: "density",
			SuiteOptionsSchema: suites.Options{
				"BatchSize":        10,
				"BatchWaitTimeout": 5 * time.Minute,
				"VMI": suites.Options{
					"ContainerDisk": "quay.io/kubevirt/cirros-container-disk-demo:latest",
					"Memory":        "90Mi",
					"CPU":           "100m",
				},
			},
		},
		&fakeSuite{
			SuiteName: "etcd",
			SuiteOptionsSchema: suites.Options{
				"EtcdEndpoints":       "https://127.0.0.1:2379",
				"EtcdMetricsPath":     "/metrics",
				"EtcdMetricsPortName": "metrics",
			},
		},
		&fakeSuite{
			SuiteName: "kbench",
			SuiteOptionsSchema: suites.Options{
				"BandwidthBlockSize":  "128K",
				"BandwidthIoDepth":    16,
				"BandwidthNumJobs":    4,
				"IOPSBlockSize":       "4K",
				"IOPSIoDepth":         32,
				"IOPSNumJobs":         4,
				"LatencyBlockSize":    "4K",
				"LatencyIoDepth":      1,
				"LatencyNumJobs":      1,
				"PVCStorageClassName": "longhorn",
			},
		},
	}

	// runOptions is a global map that stores the un-canonicalized user-provided
	// config options. it gets reset after each test case.
	t.Run("inputs conform to schema", func(t *testing.T) {
		runOptions = map[string]suites.Options{
			"global": {
				"etcdNamespace":    "etcd-system",
				"etcdReadyTimeout": 10 * time.Second,
			},
			"etcd": {
				"etcdendpoints": "https://etcd.cluster:2379",
			},
			"kbench": {
				"pvcStorageClassName": "localpath",
			},
			"density": {
				"vmi": suites.Options{
					"memory": "128Mi",
					"cpu":    "200m",
				},
			},
		}
		defer func() {
			runOptions = nil
		}()

		got, err := canonicalizeRunOptions(fakeSuites)
		if err != nil {
			t.Fatalf("canonicalizeRunOptions() returned an error: %v", err)
		}

		want := map[string]suites.Options{
			"global": {
				"EtcdNamespace":    "etcd-system",
				"EtcdReadyTimeout": 10 * time.Second,
			},
			"etcd": {
				"EtcdEndpoints": "https://etcd.cluster:2379",
			},
			"kbench": {
				"PVCStorageClassName": "localpath",
			},
			"density": {
				"VMI": suites.Options{
					"Memory": "128Mi",
					"CPU":    "200m",
				},
			},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("canonicalizeRunOptions() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("inputs have unsupported keys", func(t *testing.T) {
		runOptions = map[string]suites.Options{
			"global": {
				"foo": "bar",
			},
			"etcd": {
				"etcdEndpoints": "https://etcd.cluster:2379",
				"foo":           "bar",
			},
			"kbench": {
				"pvcStorageClassName": "localpath",
				"foo":                 "bar",
			},
			"density": {
				"foo": "bar",
				"vMI": suites.Options{
					"memory": "128Mi",
					"cpu":    "200m",
				},
			},
		}
		defer func() {
			runOptions = nil
		}()

		if _, err := canonicalizeRunOptions(fakeSuites); err == nil {
			t.Errorf("canonicalizeRunOptions() did not return an error for unsupported keys")
		}
	})
}

func TestLoadRunOptionsPreservesSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hvperf.yaml")
	config := `
global:
  etcdVersion: v3.6.0
  kbenchVersion: v1.0.0
density:
  etcdNamespace: custom-etcd
  densityDuration: "5s"
  example-suite:
    enabled: true
    suiteDuration: "10s"
    workload:
      name: sample
`
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	opts, err := loadRunOptions(path)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]suites.Options{
		"global": {
			"etcdVersion":   "v3.6.0",
			"kbenchVersion": "v1.0.0",
		},
		"density": {
			"etcdNamespace":   "custom-etcd",
			"densityDuration": "5s",
			"example-suite": suites.Options{
				"enabled":       true,
				"suiteDuration": "10s",
				"workload": suites.Options{
					"name": "sample",
				},
			},
		},
	}
	if diff := cmp.Diff(want, opts); diff != "" {
		t.Fatalf("loadRunOptions() mismatch (-want +got):\n%s", diff)
	}
}

func TestLoadRunOptionsOptionalConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := loadRunOptions("./hvperf.yaml"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("explicit missing config error = %v", err)
	}
	if got := runCmd.PersistentFlags().Lookup("config").DefValue; got != "" {
		t.Fatalf("config flag default = %q", got)
	}
}

func TestLoadRunOptionsMissingCustomPath(t *testing.T) {
	if _, err := loadRunOptions(filepath.Join(t.TempDir(), "missing.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing custom config error = %v", err)
	}
}

func TestCanonicalizeRunOptionsEdgeCases(t *testing.T) {
	fake := []suites.Suite{
		&fakeSuite{SuiteName: "etcd", SuiteOptionsSchema: suites.Options{"EtcdEndpoints": "x"}},
		&fakeSuite{SuiteName: "kbench", SuiteOptionsSchema: suites.Options{"PVCStorageClassName": "x"}},
	}

	t.Run("nil run options yield empty result", func(t *testing.T) {
		runOptions = nil
		got, err := canonicalizeRunOptions(fake)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %v, want empty", got)
		}
	})

	t.Run("unknown suite name is rejected", func(t *testing.T) {
		runOptions = map[string]suites.Options{"nosuchsuite": {"a": 1}}
		defer func() { runOptions = nil }()

		_, err := canonicalizeRunOptions(fake)
		if err == nil || !strings.Contains(err.Error(), "nosuchsuite") {
			t.Errorf("error = %v, want it to name %q", err, "nosuchsuite")
		}
	})

	t.Run("suite in run options but not selected is rejected", func(t *testing.T) {
		runOptions = map[string]suites.Options{"density": {"batchSize": 1}}
		defer func() { runOptions = nil }()

		if _, err := canonicalizeRunOptions(fake); err == nil {
			t.Error("want error for options of a suite that is not being run")
		}
	})

	t.Run("suite without user options gets empty options", func(t *testing.T) {
		runOptions = map[string]suites.Options{"etcd": {"etcdendpoints": "e"}}
		defer func() { runOptions = nil }()

		got, err := canonicalizeRunOptions(fake)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]suites.Options{
			"etcd":   {"EtcdEndpoints": "e"},
			"kbench": {},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("global options only", func(t *testing.T) {
		runOptions = map[string]suites.Options{"global": {"etcdnamespace": "ns"}}
		defer func() { runOptions = nil }()

		got, err := canonicalizeRunOptions(fake)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["global"]["EtcdNamespace"] != "ns" {
			t.Errorf("global = %v, want EtcdNamespace=ns", got["global"])
		}
	})
}

// one bad key per section, so each case fails only if that section's
// validation breaks. error must name the section.
func TestCanonicalizeRunOptionsBadKeyPerSection(t *testing.T) {
	fake := []suites.Suite{
		&fakeSuite{SuiteName: "etcd", SuiteOptionsSchema: suites.Options{"EtcdEndpoints": "x"}},
		&fakeSuite{SuiteName: "density", SuiteOptionsSchema: suites.Options{
			"VMI": suites.Options{"Memory": "x"},
		}},
	}
	testCases := []struct {
		name    string
		in      map[string]suites.Options
		wantErr string
	}{
		{"global", map[string]suites.Options{"global": {"foo": 1}}, "global"},
		{"suite", map[string]suites.Options{"etcd": {"foo": 1}}, `"etcd"`},
		{"nested", map[string]suites.Options{"density": {"vmi": suites.Options{"foo": 1}}}, `"density"`},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runOptions = tc.in
			defer func() { runOptions = nil }()

			_, err := canonicalizeRunOptions(fake)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}
