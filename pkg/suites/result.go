package suites

import (
	"fmt"
	"reflect"
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
	Params  []*SuiteParam
	RunID   string
	Results []*CaseResult
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

	passed, failed, skipped, total := s.summary()
	fmt.Fprintf(&stringBuilder, "\n=== %s: %d failed, %d passed, %d skipped (%d total)\n", s.Name, failed, passed, skipped, total)
	return stringBuilder.String()
}

func (s *SuiteResult) summary() (passed int, failed int, skipped int, total int) {
	total = len(s.Results)
	for _, result := range s.Results {
		if result.Skipped {
			skipped++
			continue
		}
		if result.Success {
			passed++
			continue
		}
		failed++
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

// CaseResult represents the result of a single test case execution.
type CaseResult struct {
	CaseName      string
	Cmds          [][]string
	DateTimeStart time.Time
	DateTimeEnd   time.Time
	Objects       []runtime.Object
	Skipped       bool
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
	if c.Skipped {
		result = "SKIPPED"
	}

	fmt.Fprintf(tab, "--- %s %s (%s)\n", result, c.CaseName, c.DateTimeEnd.Sub(c.DateTimeStart).Round(time.Millisecond))
	if c.Skipped {
		return stringBuilder.String()
	}
	fmt.Fprintf(tab, "%sStarted on:\t%s\n", indent, c.DateTimeStart.Format("2006-01-02T15:04:05Z07:00"))
	fmt.Fprintf(tab, "%sEnded at:\t%s\n", indent, c.DateTimeEnd.Format("2006-01-02T15:04:05Z07:00"))
	for i, cmd := range c.Cmds {
		label := indent + "Cmds:"
		if i > 0 {
			label = indent
		}
		fmt.Fprintf(tab, "%s\t%s\n", label, strings.Join(cmd, " "))
	}
	if c.Err != nil {
		fmt.Fprintf(tab, "%sError:\t%v\n", indent, c.Err)
	}

	for i, obj := range c.Objects {
		label := indent + "Objects:"
		if i > 0 {
			label = indent
		}
		fmt.Fprintf(tab, "%s\t%s\n", label, objectMeta(obj))
	}

	// flush the tabwriter before appending raw output, so the tabs within the raw
	// output are treated as literal tabs, not column separators
	tab.Flush()

	if trimmed := strings.TrimSpace(c.Out); trimmed != "" {
		fmt.Fprintf(&stringBuilder, "%sOutput:\n%s\n", indent, trimmed)
	}
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
		return fmt.Sprintf("(%s) %s", kind, accessor.GetName())
	}
	return fmt.Sprintf("(%s) %s/%s", kind, ns, accessor.GetName())
}
