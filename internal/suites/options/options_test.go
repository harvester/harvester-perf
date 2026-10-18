package options

import (
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/harvester/hvperf/pkg/suites"
)

type customOpts struct {
	Name    string
	Timeout time.Duration
	Count   int
}

func TestFromOptionsParsesDurationStrings(t *testing.T) {
	tests := []struct {
		name string
		opts suites.Options
		want customOpts
	}{
		{
			name: "string duration",
			opts: suites.Options{"Timeout": "10m"},
			want: customOpts{Timeout: 10 * time.Minute},
		},
		{
			name: "key case-insensitive",
			opts: suites.Options{"timeout": "90s"},
			want: customOpts{Timeout: 90 * time.Second},
		},
		{
			name: "time.Duration value passes through",
			opts: suites.Options{"Timeout": 5 * time.Second},
			want: customOpts{Timeout: 5 * time.Second},
		},
		{
			name: "duration-like string in string field is untouched",
			opts: suites.Options{"Name": "5s", "Count": 3},
			want: customOpts{Name: "5s", Count: 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FromOptions[customOpts](tt.opts)
			if err != nil {
				t.Fatalf("FromOptions() error = %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("FromOptions() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFromOptionsPointerType(t *testing.T) {
	got, err := FromOptions[*customOpts](suites.Options{"Timeout": "1h"})
	if err != nil {
		t.Fatalf("FromOptions() error = %v", err)
	}
	if got.Timeout != time.Hour {
		t.Errorf("Timeout = %v, want %v", got.Timeout, time.Hour)
	}
}

func TestFromOptionsInvalidDuration(t *testing.T) {
	_, err := FromOptions[customOpts](suites.Options{"Timeout": "10x"})
	if err == nil || !strings.Contains(err.Error(), "option Timeout") {
		t.Fatalf("FromOptions() error = %v, want error naming option Timeout", err)
	}
}

func TestFromOptionsDoesNotMutateInput(t *testing.T) {
	opts := suites.Options{"Timeout": "10m"}
	if _, err := FromOptions[customOpts](opts); err != nil {
		t.Fatalf("FromOptions() error = %v", err)
	}
	if diff := cmp.Diff(suites.Options{"Timeout": "10m"}, opts); diff != "" {
		t.Errorf("input mutated (-want +got):\n%s", diff)
	}
}
