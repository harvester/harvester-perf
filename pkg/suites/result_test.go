package suites

import (
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	// fixed timestamps keep the rendered output deterministic; UTC keeps the
	// RFC3339 offset stable regardless of the machine's local time zone
	testStart = time.Date(2026, 8, 26, 10, 30, 0, 0, time.UTC)
	testEnd   = testStart.Add(1500 * time.Millisecond)
)

func newPod(namespace, name string) *corev1.Pod {
	return &corev1.Pod{
		TypeMeta:   metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
	}
}

func TestObjectMeta(t *testing.T) {
	testCases := []struct {
		name     string
		obj      runtime.Object
		expected string
	}{
		{
			name:     "namespaced object",
			obj:      newPod("harvester-system-perf", "etcd-benchmark"),
			expected: "(Pod) harvester-system-perf/etcd-benchmark",
		},
		{
			name: "cluster scoped object has no namespace prefix",
			obj: &corev1.Node{
				TypeMeta:   metav1.TypeMeta{Kind: "Node", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: "harvester-node-0"},
			},
			expected: "(Node) harvester-node-0",
		},
		{
			name: "empty kind falls back to a placeholder",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Namespace: "harvester-system-perf", Name: "etcd-benchmark"},
			},
			expected: "(?) harvester-system-perf/etcd-benchmark",
		},
		{
			name: "empty name is rendered as-is",
			obj: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
			},
			expected: "(Pod) ",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := objectMeta(tc.obj); got != tc.expected {
				t.Errorf("objectMeta() = %q, want %q", got, tc.expected)
			}
		})
	}
}

// TestObjectMetaNoAccessor checks that an object without accessible metadata,
// such as a list, is reported instead of panicking.
func TestObjectMetaNoAccessor(t *testing.T) {
	got := objectMeta(&corev1.PodList{TypeMeta: metav1.TypeMeta{Kind: "PodList", APIVersion: "v1"}})
	if !strings.HasPrefix(got, "<unknown: ") || !strings.HasSuffix(got, ">") {
		t.Errorf("objectMeta() = %q, want a %q message", got, "<unknown: ...>")
	}
}

func TestCaseResultString(t *testing.T) {
	testCases := []struct {
		name     string
		result   *CaseResult
		expected string
	}{
		{
			name: "passing case",
			result: &CaseResult{
				CaseName:      "list-nodes",
				DateTimeStart: testStart,
				DateTimeEnd:   testEnd,
				Success:       true,
			},
			expected: "--- PASS list-nodes (1.5s)\n" +
				"    Started on:  2026-08-26T10:30:00Z\n" +
				"    Ended at:    2026-08-26T10:30:01Z\n",
		},
		{
			name: "failing case renders the error",
			result: &CaseResult{
				CaseName:      "list-nodes",
				DateTimeStart: testStart,
				DateTimeEnd:   testEnd,
				Success:       false,
				Err:           errors.New("connection refused"),
			},
			expected: "--- FAIL list-nodes (1.5s)\n" +
				"    Started on:  2026-08-26T10:30:00Z\n" +
				"    Ended at:    2026-08-26T10:30:01Z\n" +
				"    Error:       connection refused\n",
		},
		{
			name: "objects are labelled once and aligned",
			result: &CaseResult{
				CaseName:      "etcd-benchmark",
				DateTimeStart: testStart,
				DateTimeEnd:   testEnd,
				Success:       true,
				Objects: []runtime.Object{
					newPod("harvester-system-perf", "etcd-benchmark-0"),
					newPod("harvester-system-perf", "etcd-benchmark-1"),
				},
			},
			expected: "--- PASS etcd-benchmark (1.5s)\n" +
				"    Started on:  2026-08-26T10:30:00Z\n" +
				"    Ended at:    2026-08-26T10:30:01Z\n" +
				"    Objects:     (Pod) harvester-system-perf/etcd-benchmark-0\n" +
				"                 (Pod) harvester-system-perf/etcd-benchmark-1\n",
		},
		{
			name: "duration is rounded to the millisecond",
			result: &CaseResult{
				CaseName:      "list-nodes",
				DateTimeStart: testStart,
				DateTimeEnd:   testStart.Add(2*time.Second + 4*time.Microsecond),
				Success:       true,
			},
			expected: "--- PASS list-nodes (2s)\n" +
				"    Started on:  2026-08-26T10:30:00Z\n" +
				"    Ended at:    2026-08-26T10:30:02Z\n",
		},
		{
			name: "zero value case result still renders",
			result: &CaseResult{
				CaseName: "",
			},
			expected: "--- FAIL  (0s)\n" +
				"    Started on:  0001-01-01T00:00:00Z\n" +
				"    Ended at:    0001-01-01T00:00:00Z\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.result.String(); got != tc.expected {
				t.Errorf("String() =\n%q\nwant\n%q", got, tc.expected)
			}
		})
	}
}

// TestCaseResultStringLocalTimeZone checks that the timestamps carry the
// offset of the time they were recorded in, not a hard-coded UTC "Z".
func TestCaseResultStringLocalTimeZone(t *testing.T) {
	zone := time.FixedZone("CET", 2*60*60)
	c := &CaseResult{
		CaseName:      "list-nodes",
		DateTimeStart: testStart.In(zone),
		DateTimeEnd:   testEnd.In(zone),
		Success:       true,
	}

	got := c.String()
	if !strings.Contains(got, "2026-08-26T12:30:00+02:00") {
		t.Errorf("String() = %q, want it to contain the zone-local start time", got)
	}
	if !strings.Contains(got, "2026-08-26T12:30:01+02:00") {
		t.Errorf("String() = %q, want it to contain the zone-local end time", got)
	}
}

// TestCaseResultStringOutputTabs checks that tabs inside Out survive as literal
// tabs, which is why String flushes the tabwriter before appending Out.
func TestCaseResultStringOutputTabs(t *testing.T) {
	c := &CaseResult{
		CaseName:      "etcd-benchmark",
		DateTimeStart: testStart,
		DateTimeEnd:   testEnd,
		Success:       true,
		Out:           "a\tb\nlonger-cell\tc",
	}

	if got := c.String(); !strings.HasSuffix(got, "    Output:\na\tb\nlonger-cell\tc\n") {
		t.Errorf("String() = %q, want the raw output appended with its tabs intact", got)
	}
}

func TestSuiteResultSummary(t *testing.T) {
	testCases := []struct {
		name                            string
		results                         []*CaseResult
		wantPassed, wantFailed, wantTot int
	}{
		{
			name:    "no results",
			results: nil,
		},
		{
			name:       "all passed",
			results:    []*CaseResult{{Success: true}, {Success: true}},
			wantPassed: 2,
			wantTot:    2,
		},
		{
			name:       "all failed",
			results:    []*CaseResult{{Success: false}, {Success: false}},
			wantFailed: 2,
			wantTot:    2,
		},
		{
			name:       "mixed",
			results:    []*CaseResult{{Success: true}, {Success: false}, {Success: true}},
			wantPassed: 2,
			wantFailed: 1,
			wantTot:    3,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s := &SuiteResult{Name: "etcd-benchmark", RunID: "abc123", Results: tc.results}

			passed, failed, total := s.summary()
			if passed != tc.wantPassed {
				t.Errorf("summary() passed = %d, want %d", passed, tc.wantPassed)
			}
			if failed != tc.wantFailed {
				t.Errorf("summary() failed = %d, want %d", failed, tc.wantFailed)
			}
			if total != tc.wantTot {
				t.Errorf("summary() total = %d, want %d", total, tc.wantTot)
			}
			if passed+failed != total {
				t.Errorf("summary() passed+failed = %d, want total %d", passed+failed, total)
			}
		})
	}
}

func TestSuiteResultString(t *testing.T) {
	passing := &CaseResult{
		CaseName:      "list-nodes",
		DateTimeStart: testStart,
		DateTimeEnd:   testEnd,
		Success:       true,
	}
	failing := &CaseResult{
		CaseName:      "list-vms",
		DateTimeStart: testStart,
		DateTimeEnd:   testEnd,
		Success:       false,
		Err:           errors.New("connection refused"),
	}

	testCases := []struct {
		name     string
		result   *SuiteResult
		expected string
	}{
		{
			name:   "no case results",
			result: &SuiteResult{Name: "node-capacity", RunID: "abc123"},
			expected: "=== SUITE node-capacity (run abc123)\n" +
				"\n=== node-capacity: 0 failed, 0 passed (0 total)\n",
		},
		{
			name:   "single passing case",
			result: &SuiteResult{Name: "node-capacity", RunID: "abc123", Results: []*CaseResult{passing}},
			expected: "=== SUITE node-capacity (run abc123)\n" +
				passing.String() +
				"\n=== node-capacity: 0 failed, 1 passed (1 total)\n",
		},
		{
			name:   "mixed cases are joined by a newline",
			result: &SuiteResult{Name: "node-capacity", RunID: "abc123", Results: []*CaseResult{passing, failing}},
			expected: "=== SUITE node-capacity (run abc123)\n" +
				passing.String() + "\n" + failing.String() +
				"\n=== node-capacity: 1 failed, 1 passed (2 total)\n",
		},
		{
			name:   "zero value suite result still renders",
			result: &SuiteResult{},
			expected: "=== SUITE  (run )\n" +
				"\n=== : 0 failed, 0 passed (0 total)\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.result.String(); got != tc.expected {
				t.Errorf("String() =\n%q\nwant\n%q", got, tc.expected)
			}
		})
	}
}
