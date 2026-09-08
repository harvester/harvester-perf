package suites

import (
	"encoding/json"
	"fmt"
	"io"
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
	CmdResults    []*CmdResult
	DateTimeStart time.Time
	DateTimeEnd   time.Time
	MetricResults []*MetricResult
	Objects       []runtime.Object
	Skipped       bool
	Success       bool
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

	for i, r := range c.CmdResults {
		if i == 0 {
			fmt.Fprintf(tab, "%sExec:\n", indent)
		}

		fmt.Fprintf(tab, "%sCmd: %s\n", strings.Repeat(indent, 2), strings.Join(r.Cmd, " "))
		if r.Stdout != nil {
			stdout, err := io.ReadAll(r.Stdout)
			if err != nil {
				klog.V(3).ErrorS(err, "failed to read stdout", "cmd", r.Cmd)
			}
			if stdout != nil {
				if trimmed := strings.TrimSpace(string(stdout)); trimmed != "" {
					fmt.Fprintf(tab, "%sStdout: ", strings.Repeat(indent, 2))

					// tabs in stdout are replaced with 4 spaces to avoid conflicts with the
					// tabwriter output
					fmt.Fprintf(tab, "%s\n", strings.ReplaceAll(trimmed, "\t", "    "))
				}
			}
		}

		if r.Stderr != nil {
			stderr, err := io.ReadAll(r.Stderr)
			if err != nil {
				klog.V(3).ErrorS(err, "failed to read stderr for command", "cmd", r.Cmd)
			}
			if stderr != nil {
				if trimmed := strings.TrimSpace(string(stderr)); trimmed != "" {
					klog.V(3).InfoS("stderr output for command", "cmd", r.Cmd, "stderr", trimmed)
				}
			}
		}

		if r.Err != nil {
			fmt.Fprintf(tab, "%sError:\t%v\n", strings.Repeat(indent, 2), r.Err)
		}
	}

	for i, m := range c.MetricResults {
		if i == 0 {
			fmt.Fprintf(tab, "%sMetrics:\n", indent)
		}

		fmt.Fprintf(tab, "%sQuery: %s\n", strings.Repeat(indent, 2), m.Query)
		if len(m.Samples) == 0 {
			fmt.Fprintf(tab, "%sValue: N/A\n", strings.Repeat(indent, 2))
		} else {
			for _, s := range m.Samples {
				fmt.Fprintf(tab, "%sValue: %.4f\t(%s)\n", strings.Repeat(indent, 2), s.Value, s.Timestamp)
				if s.Histogram != nil {
					fmt.Fprintf(tab, "%s%s\n", strings.Repeat(indent, 2), s.Histogram)
				}
			}
		}
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
type CmdResult struct {
	Cmd    []string
	Stdout io.Reader
	Stderr io.Reader
	Err    error
}

// MarshalJSON implements the json.Marshaler interface for CmdResult. It reads the
// stdout and stderr streams and includes their contents in the JSON output.
func (c *CmdResult) MarshalJSON() ([]byte, error) {
	var stdout string
	if c.Stdout != nil {
		b, err := io.ReadAll(c.Stdout)
		if err != nil {
			return nil, err
		}

		// tabs in stdout are replaced with 4 spaces to avoid conflicts with the
		// tabwriter output
		stdout = strings.ReplaceAll(string(b), "\t", "    ")
	}

	var stderr string
	if c.Stderr != nil {
		b, err := io.ReadAll(c.Stderr)
		if err != nil {
			return nil, err
		}

		// tabs in stdout are replaced with 4 spaces to avoid conflicts with the
		// tabwriter output
		stderr = strings.ReplaceAll(string(b), "\t", "    ")
	}

	errStr := ""
	if c.Err != nil {
		errStr = c.Err.Error()
	}

	return json.Marshal(struct {
		Cmd    string
		Stdout string
		Stderr string
		Err    string
	}{
		Cmd:    strings.Join(c.Cmd, " "),
		Stdout: string(stdout),
		Stderr: string(stderr),
		Err:    errStr,
	})
}

// MetricResult represents the result of a Prometheus query executed in a test case.
type MetricResult struct {
	Query   string
	Samples model.Vector
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
