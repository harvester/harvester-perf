package suites

import (
	"fmt"
	"reflect"
	"sort"
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

// NewSuiteResult creates a new SuiteResult instance with the provided name and
// runID.
func NewSuiteResult(name string, runID string) SuiteResult {
	return SuiteResult{
		Name:  name,
		RunID: runID,
	}
}

// WithParams sets the parameters for the SuiteResult and returns the updated
// test suite result.
func (s SuiteResult) WithParams(params []*SuiteParam) SuiteResult {
	s.Params = params
	return s
}

// WithResults sets the test case results for the SuiteResult and returns the
// test suite result.
func (s SuiteResult) WithResults(results []*CaseResult) SuiteResult {
	s.Results = results
	return s
}

// WithError sets the error for the SuiteResult and returns the updated test
// suites result.
func (s SuiteResult) WithError(err error) SuiteResult {
	if err != nil {
		s.Err = err.Error()
	}
	return s
}

func (s *SuiteResult) String() string {
	var (
		stringBuilder strings.Builder
		results       []string
	)

	fmt.Fprintf(&stringBuilder, "=== SUITE %s (run %s)\n", s.Name, s.RunID)
	if SuiteErr := s.Err; SuiteErr != "" {
		fmt.Fprintf(&stringBuilder, "--- ERROR: %v\n", SuiteErr)
	}

	for _, result := range s.Results {
		results = append(results, result.String())
	}
	if len(results) > 0 {
		fmt.Fprint(&stringBuilder, strings.Join(results, "\n"))
	} else {
		fmt.Fprintf(&stringBuilder, "--- No test cases were executed.\n")
	}

	for i, param := range s.Params {
		if i == 0 {
			fmt.Fprintf(&stringBuilder, "%sParams:\n", indent)
		}
		fmt.Fprintf(&stringBuilder, "%s\t%s\n", indent, param)
	}

	passed, errored, skipped, total := s.summary()
	fmt.Fprintf(&stringBuilder, "\n=== SUMMARY: %d errored, %d passed, %d skipped (%d total)\n", errored, passed, skipped, total)
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

// CapacityResult records the maximum resource count the cluster sustained
// before a health gate or creation failure was observed.
type CapacityResult struct {
	Resource   string // e.g. "virtualmachines.kubevirt.io"
	Max        int    // last batch count confirmed healthy before failure
	Err        string // why the ramp stopped; alone does NOT cause ERROR state
	CleanupErr string // cleanup failed; causes ERROR state
}

func (c *CapacityResult) String() string {
	var str strings.Builder

	fmt.Fprintf(&str, "Resource: %s\n", c.Resource)
	fmt.Fprintf(&str, "Max: %d\n", c.Max)
	if c.Err != "" {
		fmt.Fprintf(&str, "Stopped: %s\n", c.Err)
	}
	if c.CleanupErr != "" {
		fmt.Fprintf(&str, "CleanupError: %s\n", c.CleanupErr)
	}

	return str.String()
}

// CaseResult represents the result of a single test case execution. Err captures
// errors that occur during the test case setup and cleanup.
type CaseResult struct {
	CapacityResults    []*CapacityResult
	CmdResults         []*CmdResult
	K8sResourceResults []*K8sResourceResult
	MetricResults      []*MetricResult
	PodLogResults      []*PodLogResult

	CaseName      string
	DateTimeStart time.Time
	DateTimeEnd   time.Time
	Err           string
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
// Calling NewCaseResult or one of the With* methods will recompute the finalized
// state of the CaseResult based on the errors in the CmdResults, MetricResults,
// K8sResourceResults, and the Err field.
// Caller can also explicitly call CaseResult.FinalizeState() to finalize the
// case result state.
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

// NewCaseResultErrored creates a new CaseResult instance for a test case that
// errored with err during execution.
func NewCaseResultErrored(name string, start, end time.Time, err error) *CaseResult {
	return &CaseResult{
		CaseName:      name,
		DateTimeStart: start,
		DateTimeEnd:   end,
		Err:           err.Error(),
		State:         CaseResultStateErrored,
	}
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
		return
	}

	for _, cap := range c.CapacityResults {
		if cap.CleanupErr != "" {
			c.State = CaseResultStateErrored
			return
		}
	}

	for _, cr := range c.CmdResults {
		if cr.Err != "" {
			c.State = CaseResultStateErrored
			return
		}
	}

	for _, mr := range c.MetricResults {
		if mr.Err != "" {
			c.State = CaseResultStateErrored
			return
		}
	}

	for _, kr := range c.K8sResourceResults {
		if kr.Err != "" {
			c.State = CaseResultStateErrored
			return
		}
	}

	for _, log := range c.PodLogResults {
		if log.Err != "" {
			c.State = CaseResultStateErrored
			return
		}
	}

	c.State = CaseResultStatePassed
}

func (c *CaseResult) WithK8sResourceResults(results []*K8sResourceResult) *CaseResult {
	c.K8sResourceResults = results
	c.FinalizeState()
	return c
}

func (c *CaseResult) WithPodLogResults(results []*PodLogResult) *CaseResult {
	c.PodLogResults = results
	c.FinalizeState()
	return c
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
	fmt.Fprintf(tab, "%sStarted on:\t%s\n", indent, c.DateTimeStart.Format("2006-01-02T15:04:05Z07:00"))
	fmt.Fprintf(tab, "%sEnded at:\t%s\n", indent, c.DateTimeEnd.Format("2006-01-02T15:04:05Z07:00"))
	if c.Err != "" {
		fmt.Fprintf(tab, "%sError:\t%s\n", indent, c.Err)
	}

	if c.State == CaseResultStateSkipped {
		tab.Flush()
		return strings.TrimSpace(stringBuilder.String())
	}

	resultBlocks := []string{}
	for _, r := range c.CapacityResults {
		resultBlocks = append(resultBlocks, r.String())
	}

	for _, r := range c.CmdResults {
		resultBlocks = append(resultBlocks, r.String())

		// stderr from remote command is often noisy - by default, we don't stringify
		// it, but stream it to klog at V(3) for debugging purposes
		if stderr := r.Stderr; stderr != "" {
			if trimmed := strings.TrimSpace(string(stderr)); trimmed != "" {
				klog.V(3).InfoS("[remote-exec] stderr output", "cmd", r.Cmd, "stderr", trimmed)
			}
		}
	}

	for _, r := range c.MetricResults {
		resultBlocks = append(resultBlocks, r.String())
	}

	for _, r := range c.K8sResourceResults {
		resultBlocks = append(resultBlocks, r.String())
	}

	for _, r := range c.PodLogResults {
		resultBlocks = append(resultBlocks, r.String())
	}

	if len(resultBlocks) > 0 {
		fmt.Fprintf(tab, "%sResults:\n", indent)
		fmt.Fprint(tab, strings.Join(resultBlocks, "\n"))
	}

	if len(resultBlocks) > 0 && len(c.Objects) > 0 {
		fmt.Fprint(tab, "\n")
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
	Cmd    string
	Err    string
	Node   string
	Stdout string
	Stderr string
}

func (c *CmdResult) String() string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Cmd: %s\n", c.Cmd)

	// for cases where the command is executed on a specific node, include the node
	// name in the output
	if c.Node != "" {
		fmt.Fprintf(&sb, "Node: %s\n", c.Node)
	}

	// tabs in stdout and stderr are replaced with 4 spaces to avoid conflicts with
	// the tabwriter output
	if stdout := c.Stdout; stdout != "" {
		if trimmed := strings.TrimSpace(string(stdout)); trimmed != "" {
			fmt.Fprintln(&sb, "Out:")
			fmt.Fprintf(&sb, "%s\n", strings.ReplaceAll(trimmed, "\t", "    "))
		}
	}

	if err := c.Err; err != "" {
		fmt.Fprintf(&sb, "Error: %v\n", err)
	}

	return sb.String()
}

// MetricResult represents the result of a Prometheus query executed in a test case.
// Err captures promclient errors that occur during query execution.
type MetricResult struct {
	Err      string
	Query    string
	Samples  model.Vector
	Warnings []string
}

func (m *MetricResult) String() string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Query: %s\n", m.Query)
	for _, s := range m.Samples {
		fmt.Fprintf(&sb, "Value: %v\n", s)
	}

	if m.Err != "" {
		fmt.Fprintf(&sb, "Error: %v\n", m.Err)
	}

	if len(m.Warnings) > 0 {
		fmt.Fprintln(&sb, "Warnings:")
		for _, w := range m.Warnings {
			fmt.Fprintf(&sb, "- %s\n", w)
		}
	}

	return sb.String()
}

// K8sResourceResult represents the result of querying Kubernetes resources
// in a test case. Data is a map of key-value pairs representing the resource's
// spec or status attributes and values, while Err captures any errors that occur
// during the query.
type K8sResourceResult struct {
	Resource string
	Subject  string
	Data     map[string]string
	Err      string
}

func (k *K8sResourceResult) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Name: %s/%s\n", k.Resource, k.Subject)

	if len(k.Data) > 0 {
		fmt.Fprintln(&sb, "Data:")
		var sortedByKeys []string
		for key, value := range k.Data {
			sortedByKeys = append(sortedByKeys, fmt.Sprintf("%s: %s", key, value))
		}
		sort.Strings(sortedByKeys)
		for _, s := range sortedByKeys {
			fmt.Fprintf(&sb, "%s\n", s)
		}
	}

	if k.Err != "" {
		fmt.Fprintf(&sb, "Error: %v\n", k.Err)
	}

	return sb.String()
}

// PodLogResult represents the result of querying logs from a Kubernetes pod.
type PodLogResult struct {
	Err     string
	Logs    string
	Subject string
}

func (p *PodLogResult) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Name: %s\n", p.Subject)

	if out := p.Logs; out != "" {
		fmt.Fprintln(&sb, "Logs:")
		fmt.Fprint(&sb, out)
	}

	if p.Err != "" {
		fmt.Fprintf(&sb, "Error: %v\n", p.Err)
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
