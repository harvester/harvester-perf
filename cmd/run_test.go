package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/harvester/hvperf/pkg/suites"
)

func TestLoadRunOptionsOverridesGlobalsAndPreservesSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hvperf.yaml")
	config := `
density:
  EtcdNamespace: custom-etcd
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
		"density": {
			"EtcdNamespace":   "custom-etcd",
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
