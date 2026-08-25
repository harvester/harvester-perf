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
	fmt.Fprintf(&b, "Name:%s\n", s.Name)
	fmt.Fprintf(&b, "Name:%s\n", s.RunID)

	var sr []string
	for _, result := range s.Results {
		sr = append(sr, result.String())
	}

	fmt.Fprint(&b, strings.Join(sr, "\n"))
	return b.String()
}

// CaseResult represents the result of a single test case execution.
type CaseResult struct {
	Description string
	ObjMeta     []metav1.Object
	Success     bool
	Out         string
	Err         error
}

func (c *CaseResult) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Description: %s\n", c.Description)

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

	if c.Out != "" {
		fmt.Fprintf(&b, "\n[stdout]\n%s", c.Out)
	}
	if c.Err != nil {
		fmt.Fprintf(&b, "\n[err]\n%s", c.Err)
	}

	return b.String()
}
