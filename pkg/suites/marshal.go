package suites

import (
	"encoding/json"
	"fmt"
)

// SuiteMarshaler is a wrapper around the Suite interface that provides a custom
// JSON and YAML marshaling implementation for the concrete types of Suite
// implementation. This helps to avoid duplicating metadata fields in the Suite
// concrete types. Every Suite implementation should embed this struct to ensure
// that the suite can be marshaled correctly.
type SuiteMarshaler struct {
	Marshal Suite
}

// String returns a string representation of the SuiteMarshaler.
func (t *SuiteMarshaler) String() string {
	if t.Marshal.Name() == "" {
		return ""
	}
	rw := "read-only"
	if t.Marshal.IsReadWrite() {
		rw = "read-write"
	}
	return fmt.Sprintf("%s\t%s\t%s", t.Marshal.Name(), rw, t.Marshal.Description())
}

type SuiteMarshal struct {
	Name        string `json:"name"`
	IsReadWrite bool   `json:"readWrite"`
	Description string `json:"description"`
}

// MarshalJSON implements the json.Marshaler interface for SuiteMarshaler.
func (t *SuiteMarshaler) MarshalJSON() ([]byte, error) {
	return json.Marshal(SuiteMarshal{
		Name:        t.Marshal.Name(),
		Description: t.Marshal.Description(),
		IsReadWrite: t.Marshal.IsReadWrite(),
	})
}

// MarshalYAML implements the yaml.Marshaler interface for SuiteMarshaler.
func (t *SuiteMarshaler) MarshalYAML() (any, error) {
	return SuiteMarshal{
		Name:        t.Marshal.Name(),
		Description: t.Marshal.Description(),
		IsReadWrite: t.Marshal.IsReadWrite(),
	}, nil
}
