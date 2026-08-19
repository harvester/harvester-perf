package suites

import "fmt"

// TestSuite defines the interface that every test suite must implement.
type TestSuite interface {
	Name() string
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
	rw := "read-only"
	if t.Marshal.IsReadWrite() {
		rw = "read-write"
	}
	return fmt.Sprintf("%s\t%s", t.Marshal.Name(), rw)
}

// MarshalJSON implements the json.Marshaler interface for TestSuiteMarshaler.
func (t *TestSuiteMarshaler) MarshalJSON() ([]byte, error) {
	raw := fmt.Sprintf(`{"name": "%s", "readWrite": %t}`, t.Marshal.Name(), t.Marshal.IsReadWrite())
	return []byte(raw), nil
}

// MarshalYAML implements the yaml.Marshaler interface for TestSuiteMarshaler.
func (t *TestSuiteMarshaler) MarshalYAML() (any, error) {
	return struct {
		Name        string `yaml:"name"`
		IsReadWrite bool   `yaml:"readWrite"`
	}{
		Name:        t.Marshal.Name(),
		IsReadWrite: t.Marshal.IsReadWrite(),
	}, nil
}
