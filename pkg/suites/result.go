package suites

// SuiteResult represents the result of a test suite execution.
type SuiteResult struct {
	TestSuiteName string
	TestRunID     string
	Out           string
	Err           string
}
