package suites

import "fmt"

// SuiteResult represents the result of a test suite execution.
type SuiteResult struct {
	TestSuiteName string
	TestRunID     string
	IsSuccess     bool
	Out           string
	Err           string
}

func (r SuiteResult) String() string {
	state := "success"
	if !r.IsSuccess {
		state = "failed"
	}
	s := fmt.Sprintf("Name:%s\t(%s)", r.TestSuiteName, state)
	if r.Out != "" {
		s += fmt.Sprintf("\n[stdout]\n%s", r.Out)
	}
	if r.Err != "" {
		s += fmt.Sprintf("\n[stderr]\n%s", r.Out)
	}

	return s
}
