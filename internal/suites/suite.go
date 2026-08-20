package suites

import (
	"encoding/json"
	"fmt"
)

// TestSuite defines the interface that every test suite must implement.
type TestSuite interface {
	Name() string
	Description() string
	IsReadWrite() bool
	RunE() error
}

// TestSuiteMarshaler is a wrapper around TestSuite that provides a custom JSON
// and YAML marshaling implementation. Every TestSuite implementation should
// embed this struct to ensure that the suite can be marshaled correctly.
type TestSuiteMarshaler struct {
	Marshal TestSuite
}

// String returns a string representation of the TestSuiteMarshaler.
func (t *TestSuiteMarshaler) String() string {
	if t.Marshal.Name() == "" {
		return ""
	}
	rw := "read-only"
	if t.Marshal.IsReadWrite() {
		rw = "read-write"
	}
	return fmt.Sprintf("%s\t%s\t%s", t.Marshal.Name(), rw, t.Marshal.Description())
}

type SuiteOutput struct {
	Name        string `json:"name"`
	IsReadWrite bool   `json:"readWrite"`
	Description string `json:"description"`
}

// MarshalJSON implements the json.Marshaler interface for TestSuiteMarshaler.
func (t *TestSuiteMarshaler) MarshalJSON() ([]byte, error) {
	return json.Marshal(SuiteOutput{
		Name:        t.Marshal.Name(),
		Description: t.Marshal.Description(),
		IsReadWrite: t.Marshal.IsReadWrite(),
	})
}

// MarshalYAML implements the yaml.Marshaler interface for TestSuiteMarshaler.
func (t *TestSuiteMarshaler) MarshalYAML() (any, error) {
	return SuiteOutput{
		Name:        t.Marshal.Name(),
		Description: t.Marshal.Description(),
		IsReadWrite: t.Marshal.IsReadWrite(),
	}, nil
}
