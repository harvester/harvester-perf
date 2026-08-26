package suites

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
)

const indent = "    "

// SuiteResult represents the result of a test suite execution.
type SuiteResult struct {
	Name    string
	RunID   string
	Results []*CaseResult
}

func (s *SuiteResult) String() string {
	var (
		stringBuilder strings.Builder
		results       []string
	)

	fmt.Fprintf(&stringBuilder, "=== SUITE %s (run %s)\n", s.Name, s.RunID)
	for _, result := range s.Results {
		results = append(results, result.String())
	}
	fmt.Fprint(&stringBuilder, strings.Join(results, "\n"))

	passed, failed, total := s.summary()
	fmt.Fprintf(&stringBuilder, "\n=== %s: %d failed, %d passed (%d total)\n", s.Name, failed, passed, total)
	return stringBuilder.String()
}

func (s *SuiteResult) summary() (passed int, failed int, total int) {
	total = len(s.Results)
	for _, result := range s.Results {
		if result.Success {
			passed++
			continue
		}
		failed++
	}
	return
}

// CaseResult represents the result of a single test case execution.
type CaseResult struct {
	CaseName      string
	DateTimeStart time.Time
	DateTimeEnd   time.Time
	Objects       []runtime.Object
	Success       bool
	Out           string
	Err           error
}

func (c *CaseResult) String() string {
	var (
		stringBuilder strings.Builder
		tab           = tabwriter.NewWriter(&stringBuilder, 0, 0, 2, ' ', 0)
	)

	result := "PASS"
	if !c.Success {
		result = "FAIL"
	}
	fmt.Fprintf(tab, "--- %s %s (%s)\n", result, c.CaseName, c.DateTimeEnd.Sub(c.DateTimeStart).Round(time.Millisecond))
	fmt.Fprintf(tab, "%sStarted on:\t%s\n", indent, c.DateTimeStart.Format("2006-01-02T15:04:05Z07:00"))
	fmt.Fprintf(tab, "%sEnded at:\t%s\n", indent, c.DateTimeEnd.Format("2006-01-02T15:04:05Z07:00"))
	fmt.Fprintf(tab, "%sError:\t%v\n", indent, c.Err)

	for i, obj := range c.Objects {
		label := indent + "Objects:"
		if i > 0 {
			label = indent
		}
		fmt.Fprintf(tab, "%s\t%s\n", label, objectMeta(obj))
	}

	fmt.Fprintf(tab, "%sOutput:\n%s\n", indent, strings.TrimSpace(c.Out))
	tab.Flush()
	return stringBuilder.String()
}

// objectMeta renders "(kind) namespace/name" for the Objects list in
// suite output, tolerating objects that don't carry accessible metadata.
func objectMeta(obj runtime.Object) string {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return fmt.Sprintf("<unknown: %v>", err)
	}
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	if kind == "" {
		kind = "?"
	}
	ns := accessor.GetNamespace()
	if ns == "" {
		return fmt.Sprintf("%s (%s)", kind, accessor.GetName())
	}
	return fmt.Sprintf("(%s) %s/%s", kind, ns, accessor.GetName())
}
