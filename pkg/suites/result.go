package suites

import (
	"fmt"
	"reflect"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/prometheus/common/model"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
)

const indent = "    "

// SuiteResult represents the result of a test suite execution. Err captures
// errors that occur during the suite setup and cleanup.
type SuiteResult struct {
	Name    string
	Params  []*SuiteParam
	RunID   string
	Results []*CaseResult
	Err     string
}

func (s *SuiteResult) String() string {
	var (
		stringBuilder strings.Builder
		results       []string
	)

	fmt.Fprintf(&stringBuilder, "=== SUITE %s (run %s)\n", s.Name, s.RunID)
	for i, param := range s.Params {
		if i == 0 {
			fmt.Fprintf(&stringBuilder, "%sParams:\n", indent)
		}
		fmt.Fprintf(&stringBuilder, "%s\t%s\n", indent, param)
	}

	for _, result := range s.Results {
		results = append(results, result.String())
	}
	fmt.Fprint(&stringBuilder, strings.Join(results, "\n"))

	if SuiteErr := s.Err; SuiteErr != "" {
		fmt.Fprintf(&stringBuilder, "\n--- SUITE ERROR: %v\n", SuiteErr)
	}

	passed, errored, skipped, total := s.summary()
	fmt.Fprintf(&stringBuilder, "\n=== %s: %d errored, %d passed, %d skipped (%d total)\n", s.Name, errored, passed, skipped, total)
	return stringBuilder.String()
}

func (s *SuiteResult) summary() (passed int, errored int, skipped int, total int) {
	total = len(s.Results)
	for _, result := range s.Results {
		switch result.State {
		case CaseResultStatePassed:
			passed++
		case CaseResultStateErrored:
			errored++
		case CaseResultStateSkipped:
			skipped++
		}
	}
	return
}

// SuiteParam represents the parameters used to configure a test suite. They are
// derived from the suite's internal options struct, used for reporting purposes only.
type SuiteParam struct {
	Key   string
	Value string
}

func (s *SuiteParam) String() string {
	return fmt.Sprintf("%s=%s", s.Key, s.Value)
}

// ToSuiteParams converts a struct of options into a slice of SuiteParams for
// reporting purposes. For example, in the `etcd#BenchmarkSuite`, opts would be
// the `etcd#BenchmarkSuiteOptions` struct.
func ToSuiteParams(opts any) ([]*SuiteParam, error) {
	val := reflect.ValueOf(opts)
	for val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return nil, nil
		}
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil, fmt.Errorf("can't generate []SuiteParams. expected a struct, got %s", val.Kind())
	}

	valType := val.Type()
	params := []*SuiteParam{}
	for i := 0; i < valType.NumField(); i++ {
		field := valType.Field(i)
		if !field.IsExported() {
			continue
		}
		params = append(params, &SuiteParam{
			Key:   field.Name,
			Value: fmt.Sprintf("%v", val.Field(i)),
		})
	}
	return params, nil
}

// CaseResult represents the result of a single test case execution. Err captures
// errors that occur during the test case setup and cleanup.
type CaseResult struct {
	CaseName      string
	CmdResults    []*CmdResult
	DateTimeStart time.Time
	DateTimeEnd   time.Time
	Err           error
	MetricResults []*MetricResult
	Objects       []runtime.Object
	State         CaseResultState
}

type CaseResultState string

const (
	CaseResultStatePassed  CaseResultState = "PASS"
	CaseResultStateErrored CaseResultState = "ERROR"
	CaseResultStateSkipped CaseResultState = "SKIP"
	CaseResultStateUnknown CaseResultState = "UNKNOWN"
)

// NewCaseResult creates a new CaseResult instance with the provided parameters.
// Typically, NewCaseResult is called after a test case has been executed, to
// finalize its state based on the command and metric results.
// If NewCaseResult is called before a test case is executed, caller has to
// explicitly call CaseResult.FinalizeState() to finalize the case result state.
func NewCaseResult(
	name string,
	start time.Time,
	end time.Time,
	cmdResults []*CmdResult,
	metricResults []*MetricResult,
	objs ...runtime.Object,
) *CaseResult {
	cr := &CaseResult{
		CaseName:      name,
		CmdResults:    cmdResults,
		MetricResults: metricResults,
		DateTimeStart: start,
		DateTimeEnd:   end,
		Objects:       objs,
		State:         CaseResultStateUnknown,
	}
	cr.FinalizeState()

	return cr
}

// NewCaseResultSkipped creates a new CaseResult instance for a test case that
// is skipped with an error during execution.
func NewCaseResultSkipped(name string, start, end time.Time, err error) *CaseResult {
	return &CaseResult{
		CaseName:      name,
		DateTimeStart: start,
		DateTimeEnd:   end,
		Err:           err.Error(),
		State:         CaseResultStateSkipped,
	}
}

// FinalizeState determines the final state of the CaseResult based on its
// CmdResults, MetricResults, and any errors that may have occurred during the
// test case execution.
func (c *CaseResult) FinalizeState() {
	// do not override the 'skipped' state as this is set by the test case itself
	if c.State == CaseResultStateSkipped {
		return
	}

	if c.Err != "" {
		c.State = CaseResultStateErrored
	}

	var hasErr bool
	for _, cr := range c.CmdResults {
		if cr.Err != "" {
			c.State = CaseResultStateErrored
			hasErr = true
			break
		}
	}

	for _, mr := range c.MetricResults {
		if mr.Err != "" {
			c.State = CaseResultStateErrored
			hasErr = true
			break
		}
	}

	if !hasErr {
		c.State = CaseResultStatePassed
	}
}

func (c *CaseResult) String() string {
	var (
		stringBuilder strings.Builder
		tab           = tabwriter.NewWriter(&stringBuilder, 0, 0, 2, ' ', 0)
	)

	if c.State == "" {
		c.State = CaseResultStateUnknown
	}
	fmt.Fprintf(tab, "--- %s %s (%s)\n", c.State, c.CaseName, c.DateTimeEnd.Sub(c.DateTimeStart).Round(time.Millisecond))
	if c.Err != nil {
		fmt.Fprintf(tab, "%sError:\t%v\n", indent, c.Err)
	}
	if c.Skipped {
		return stringBuilder.String()
	}

	fmt.Fprintf(tab, "%sStarted on:\t%s\n", indent, c.DateTimeStart.Format("2006-01-02T15:04:05Z07:00"))
	fmt.Fprintf(tab, "%sEnded at:\t%s\n", indent, c.DateTimeEnd.Format("2006-01-02T15:04:05Z07:00"))

	for i, r := range c.CmdResults {
		if i == 0 {
			fmt.Fprintf(tab, "%sExec:\n", indent)
		}
		r.indent = strings.Repeat(indent, 2)
		fmt.Fprintf(tab, "%s", r)

		// stderr from remote command is often noisy - by default, we don't stringify
		// it, but stream it to klog at V(3) for debugging purposes
		if stderr := r.Stderr; stderr != "" {
			if trimmed := strings.TrimSpace(string(stderr)); trimmed != "" {
				klog.V(3).InfoS("[remote-exec] stderr output", "cmd", r.Cmd, "stderr", trimmed)
			}
		}
	}

	for i, m := range c.MetricResults {
		if i == 0 {
			fmt.Fprintf(tab, "%sMetrics:\n", indent)
		}
		m.indent = strings.Repeat(indent, 2)
		fmt.Fprintf(tab, "%s", m)
	}

	for i, obj := range c.Objects {
		label := indent + "Objects:"
		if i > 0 {
			label = indent
		}
		fmt.Fprintf(tab, "%s\t%s\n", label, objectMeta(obj))
	}

	//nolint:errcheck
	tab.Flush()
	return stringBuilder.String()
}

// CmdResult represents the result of executing a command in a test case.
// Err captures errors that occur during command execution, while Stdout and
// Stderr capture the command's output streams.
type CmdResult struct {
	Cmd            string
	Stdout         string
	Stderr         string
	Err            error
	indent         string
	stderrtostring bool
}

func (c *CmdResult) String() string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "%sCmd: %s\n", c.indent, c.Cmd)
	// tabs in stdout and stderr are replaced with 4 spaces to avoid conflicts with
	// the tabwriter output
	if stdout := c.Stdout; stdout != "" {
		if trimmed := strings.TrimSpace(string(stdout)); trimmed != "" {
			fmt.Fprintf(&sb, "%sStdout: ", c.indent)
			fmt.Fprintf(&sb, "%s\n", strings.ReplaceAll(trimmed, "\t", "    "))
		}
	}

	// stderr from remote command is often noisy - by default, we don't stringify it
	if stderr := c.Stderr; stderr != "" && c.stderrtostring {
		if trimmed := strings.TrimSpace(string(stderr)); trimmed != "" {
			fmt.Fprintf(&sb, "%sStderr: ", c.indent)
			fmt.Fprintf(&sb, "%s\n", strings.ReplaceAll(trimmed, "\t", "    "))
		}
	}

	if err := c.Err; err != nil {
		fmt.Fprintf(&sb, "%sError:\t%v\n", c.indent, err)
	}

	return sb.String()
}

// MetricResult represents the result of a Prometheus query executed in a test case.
// Err captures promclient errors that occur during query execution.
type MetricResult struct {
	Err      error
	Query    string
	Samples  model.Vector
	Warnings []string
	indent   string
}

func (m *MetricResult) String() string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "%sQuery: %s\n", m.indent, m.Query)
	for _, s := range m.Samples {
		fmt.Fprintf(&sb, "%sValue: %v\n", m.indent, s)
	}

	if m.Err != nil {
		fmt.Fprintf(&sb, "%sError:\t%v\n", m.indent, m.Err)
	}

	if len(m.Warnings) > 0 {
		fmt.Fprintf(&sb, "%sWarnings:\n", m.indent)
		for _, w := range m.Warnings {
			fmt.Fprintf(&sb, "%s- %s\n", m.indent, w)
		}
	}

	return sb.String()
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
		return fmt.Sprintf("(%s) %s", kind, accessor.GetName())
	}
	return fmt.Sprintf("(%s) %s/%s", kind, ns, accessor.GetName())
}
