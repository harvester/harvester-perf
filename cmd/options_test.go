package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/harvester/hvperf/pkg/suites"
)

func TestLoadRunOptions(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		want    map[string]suites.Options
		wantErr bool
	}{
		{
			name:   "empty file",
			config: "",
			want:   map[string]suites.Options{},
		},
		{
			name: "single suite",
			config: `
kbench:
  mode: iops
`,
			want: map[string]suites.Options{
				"kbench": {
					"mode": "iops",
				},
			},
		},
		{
			name: "multiple suites",
			config: `
kbench:
  mode: iops
  testSize: 10G
etcd:
  putLoadSize: 500
  etcdEndpoints: https://10.0.0.1:2379
density:
  maxVMs: 5
  vmi:
    cpu: 200m
`,
			want: map[string]suites.Options{
				"kbench": {
					"mode":     "iops",
					"testSize": "10G",
				},
				"etcd": {
					"putLoadSize":   500,
					"etcdEndpoints": "https://10.0.0.1:2379",
				},
				"density": {
					"maxVMs": 5,
					"vmi":    suites.Options{"cpu": "200m"},
				},
			},
		},
		{
			name: "suite with no options",
			config: `
kbench:
etcd:
  putLoadSize: 500
`,
			want: map[string]suites.Options{
				"kbench": nil,
				"etcd": {
					"putLoadSize": 500,
				},
			},
		},
		{
			name:    "top-level value is not a map",
			config:  "kbench: just-a-string\n",
			wantErr: true,
		},
		{
			name:    "invalid yaml",
			config:  "kbench: [unclosed\n",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "hvperf.yaml")
			if err := os.WriteFile(path, []byte(tt.config), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := loadRunOptions(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("loadRunOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("loadRunOptions() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
