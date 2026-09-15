package suites

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestProgressReporterSuiteStart(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgressReporter(&buf, "run-123", 3)

	p.SuiteStart("etcd-benchmark", 1)

	got := strings.TrimSpace(buf.String())
	want := "[1/3] etcd-benchmark: running... (id=run-123)"
	if got != want {
		t.Errorf("SuiteStart() output = %q, want it to be %q", got, want)
	}
}

func TestProgressReporterSuiteDone(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgressReporter(&buf, "run-123", 3)

	p.SuiteDone("etcd-benchmark", 1, 2500*time.Millisecond)

	got := strings.TrimSpace(buf.String())
	want := "[1/3] etcd-benchmark: finished (2.5s)"
	if got != want {
		t.Errorf("SuiteDone() output = %q, want it to be %q", got, want)
	}
}

func TestProgressReporterCaseStart(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgressReporter(&buf, "run-123", 3)

	p.CaseStart("etcd-benchmark", "etcd healthcheck")

	got := buf.String()
	for _, want := range []string{"etcd-benchmark", "etcd healthcheck", "running..."} {
		if !strings.Contains(got, want) {
			t.Errorf("CaseStart() output = %q, want it to contain %q", got, want)
		}
	}
}

func TestProgressReporterCaseDone(t *testing.T) {
	testCases := []struct {
		name       string
		state      CaseResultState
		wantMark   string
		wantNoMark string
	}{
		{
			name:       "passed",
			state:      CaseResultStatePassed,
			wantMark:   "✓",
			wantNoMark: "✗",
		},
		{
			name:       "failed",
			state:      CaseResultStateErrored,
			wantMark:   "✗",
			wantNoMark: "✓",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := NewProgressReporter(&buf, "run-123", 3)

			p.CaseDone("etcd-benchmark", "etcd healthcheck", tc.state, 150*time.Millisecond)

			got := buf.String()
			for _, want := range []string{"etcd-benchmark", "etcd healthcheck", tc.wantMark, "150ms"} {
				if !strings.Contains(got, want) {
					t.Errorf("CaseDone() output = %q, want it to contain %q", got, want)
				}
			}
			if strings.Contains(got, tc.wantNoMark) {
				t.Errorf("CaseDone() output = %q, want it not to contain %q", got, tc.wantNoMark)
			}
		})
	}
}
