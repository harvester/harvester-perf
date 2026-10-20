package suites

import (
	"encoding/json"
	"testing"

	"go.yaml.in/yaml/v4"
)

func TestMarshalerString(t *testing.T) {
	testCases := []struct {
		name        string
		description string
		suiteName   string
		exclusive   bool
		expected    string
	}{
		{
			name:        "non-exclusive suite",
			description: "fake test suite",
			suiteName:   "node-capacity",
			exclusive:   false,
			expected:    "node-capacity\tfalse\tfake test suite",
		},
		{
			name:        "exclusive suite",
			description: "fake test suite",
			suiteName:   "etcd-benchmark",
			exclusive:   true,
			expected:    "etcd-benchmark\ttrue\tfake test suite",
		},
		{
			name:        "empty suite name",
			description: "",
			suiteName:   "",
			exclusive:   false,
			expected:    "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := &SuiteMarshaler{Marshal: newFakeSuite(tc.suiteName, tc.description, tc.exclusive)}
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
		exclusive   bool
		expected    string
	}{
		{
			name:        "non-exclusive suite",
			description: "fake test suite",
			suiteName:   "node-capacity",
			exclusive:   false,
			expected:    `{"name":"node-capacity","exclusive":false,"description":"fake test suite"}`,
		},
		{
			name:        "exclusive suite",
			description: "fake test suite",
			suiteName:   "etcd-benchmark",
			exclusive:   true,
			expected:    `{"name":"etcd-benchmark","exclusive":true,"description":"fake test suite"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := &SuiteMarshaler{Marshal: newFakeSuite(tc.suiteName, tc.description, tc.exclusive)}

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
			if decoded.IsExclusive != tc.exclusive {
				t.Errorf("decoded readWrite = %t, want %t", decoded.IsExclusive, tc.exclusive)
			}
		})
	}
}

func TestMarshalerMarshalYAML(t *testing.T) {
	testCases := []struct {
		name        string
		description string
		suiteName   string
		exclusive   bool
	}{
		{
			name:        "exclusive suite",
			description: "fake test suite",
			suiteName:   "node-capacity",
			exclusive:   false,
		},
		{
			name:        "exclusive suite",
			description: "fake test suite",
			suiteName:   "etcd-benchmark",
			exclusive:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := &SuiteMarshaler{Marshal: newFakeSuite(tc.suiteName, tc.description, tc.exclusive)}
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
			if decoded.IsExclusive != tc.exclusive {
				t.Errorf("decoded readWrite = %t, want %t", decoded.IsExclusive, tc.exclusive)
			}
		})
	}
}
