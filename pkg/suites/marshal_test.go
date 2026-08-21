package suites

import (
	"encoding/json"
	"testing"

	"go.yaml.in/yaml/v4"
)

var _ Suite = &fakeSuite{}

// fakeSuite is a minimal TestSuite implementation used to exercise
// TestSuiteMarshaler in isolation from the real test suites.
type fakeSuite struct {
	SuiteMarshaler

	name        string
	description string
	readWrite   bool
}

func newFakeSuite(name, description string, readWrite bool) *fakeSuite {
	s := &fakeSuite{name: name, description: description, readWrite: readWrite}
	s.Marshal = s
	return s
}

func (s *fakeSuite) Name() string {
	return s.name
}

func (s *fakeSuite) Description() string {
	return s.description
}

func (s *fakeSuite) IsReadWrite() bool {
	return s.readWrite
}

func (s *fakeSuite) RunE() (SuiteResult, error) {
	return SuiteResult{}, nil
}

func TestMarshalerString(t *testing.T) {
	testCases := []struct {
		name        string
		description string
		suiteName   string
		readWrite   bool
		expected    string
	}{
		{
			name:        "read-only suite",
			description: "fake test suite",
			suiteName:   "node-capacity",
			readWrite:   false,
			expected:    "node-capacity\tread-only\tfake test suite",
		},
		{
			name:        "read-write suite",
			description: "fake test suite",
			suiteName:   "etcd-benchmark",
			readWrite:   true,
			expected:    "etcd-benchmark\tread-write\tfake test suite",
		},
		{
			name:        "empty suite name",
			description: "",
			suiteName:   "",
			readWrite:   false,
			expected:    "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := &SuiteMarshaler{Marshal: newFakeSuite(tc.suiteName, tc.description, tc.readWrite)}
			if got := m.String(); got != tc.expected {
				t.Errorf("String() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestMarshalerMarshalJSON(t *testing.T) {
	testCases := []struct {
		name        string
		description string
		suiteName   string
		readWrite   bool
		expected    string
	}{
		{
			name:        "read-only suite",
			description: "fake test suite",
			suiteName:   "node-capacity",
			readWrite:   false,
			expected:    `{"name":"node-capacity","readWrite":false,"description":"fake test suite"}`,
		},
		{
			name:        "read-write suite",
			description: "fake test suite",
			suiteName:   "etcd-benchmark",
			readWrite:   true,
			expected:    `{"name":"etcd-benchmark","readWrite":true,"description":"fake test suite"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := &SuiteMarshaler{Marshal: newFakeSuite(tc.suiteName, tc.description, tc.readWrite)}

			// MarshalJSON returns an SuiteOutput struct, so assert on the encoded
			// form rather than on the concrete type.
			data, err := m.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON() returned unexpected error: %v", err)
			}
			if string(data) != tc.expected {
				t.Errorf("MarshalJSON() = %s, want %s", data, tc.expected)
			}

			// The output must also be valid JSON that decodes to the same values.
			var decoded SuiteMarshal
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("MarshalJSON() produced invalid JSON %s: %v", data, err)
			}
			if decoded.Name != tc.suiteName {
				t.Errorf("decoded name = %q, want %q", decoded.Name, tc.suiteName)
			}
			if decoded.Description != tc.description {
				t.Errorf("decoded name = %q, want %q", decoded.Description, tc.description)
			}
			if decoded.IsReadWrite != tc.readWrite {
				t.Errorf("decoded readWrite = %t, want %t", decoded.IsReadWrite, tc.readWrite)
			}
		})
	}
}

func TestMarshalerMarshalYAML(t *testing.T) {
	testCases := []struct {
		name        string
		description string
		suiteName   string
		readWrite   bool
	}{
		{
			name:        "read-only suite",
			description: "fake test suite",
			suiteName:   "node-capacity",
			readWrite:   false,
		},
		{
			name:        "read-write suite",
			description: "fake test suite",
			suiteName:   "etcd-benchmark",
			readWrite:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := &SuiteMarshaler{Marshal: newFakeSuite(tc.suiteName, tc.description, tc.readWrite)}
			v, err := m.MarshalYAML()
			if err != nil {
				t.Fatalf("MarshalYAML() returned unexpected error: %v", err)
			}

			data, err := yaml.Marshal(v)
			if err != nil {
				t.Fatalf("yaml.Marshal() returned unexpected error: %v", err)
			}

			var decoded SuiteMarshal
			if err := yaml.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("yaml.Unmarshal() returned unexpected error: %v", err)
			}
			if decoded.Name != tc.suiteName {
				t.Errorf("decoded name = %q, want %q", decoded.Name, tc.suiteName)
			}
			if decoded.Description != tc.description {
				t.Errorf("decoded name = %q, want %q", decoded.Description, tc.description)
			}
			if decoded.IsReadWrite != tc.readWrite {
				t.Errorf("decoded readWrite = %t, want %t", decoded.IsReadWrite, tc.readWrite)
			}
		})
	}
}
