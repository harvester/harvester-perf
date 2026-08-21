package suites

// Suite defines the interface that every test suite must implement.
type Suite interface {
	Name() string
	Description() string
	IsReadWrite() bool
	RunE() (SuiteResult, error)
}

// SuteOption defines the interface that every test suite option must implement.
// It is used to bind the option to a specific test suite.
type SuiteOption interface {
	Bind(suite Suite)
}
