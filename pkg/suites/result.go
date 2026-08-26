package suites

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SuiteResult represents the result of a test suite execution.
type SuiteResult struct {
	Name    string
	RunID   string
	Results []*CaseResult
}

func (s *SuiteResult) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Suite Name: %s\n", s.Name)
	fmt.Fprintf(&b, "Run ID: %s\n", s.RunID)

	var sr []string
	for _, result := range s.Results {
		sr = append(sr, result.String())
	}

	fmt.Fprint(&b, strings.Join(sr, "\n"))
	return b.String()
}

// CaseResult represents the result of a single test case execution.
type CaseResult struct {
	CaseName string
	ObjMeta  []metav1.Object
	Success  bool
	Out      string
	Err      error
}

func (c *CaseResult) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Case Name: %s\n", c.CaseName)

	result := "success"
	if !c.Success {
		result = "failed"
	}
	fmt.Fprintf(&b, "Result: %s", result)

	var metaStr []string
	for _, meta := range c.ObjMeta {
		metaStr = append(metaStr, fmt.Sprintf("  - %s/%s", meta.GetNamespace(), meta.GetName()))
	}
	if len(metaStr) > 0 {
		fmt.Fprintf(&b, "\nObjectMeta:\n%s", strings.Join(metaStr, "\n"))
	}

	if c.Err != nil {
		fmt.Fprintf(&b, "\nerr:\n%s", c.Err)
	}
	if c.Out != "" {
		fmt.Fprintf(&b, "\n\n%s", strings.TrimSpace(c.Out))
	}

	return b.String()
}
