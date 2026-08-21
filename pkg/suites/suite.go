package suites

// Suite defines the interface that every test suite must implement.
type Suite interface {
	Name() string
	Description() string
	IsReadWrite() bool
	RunE(opt Options) (SuiteResult, error)
}

type Options map[string]any
