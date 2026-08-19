package suites

import (
	"encoding/json"
	"testing"

	"go.yaml.in/yaml/v4"
)

var _ TestSuite = &fakeSuite{}

// fakeSuite is a minimal TestSuite implementation used to exercise
// TestSuiteMarshaler in isolation from the real test suites.
type fakeSuite struct {
	TestSuiteMarshaler

	name      string
	readWrite bool
}

func newFakeSuite(name string, readWrite bool) *fakeSuite {
	s := &fakeSuite{name: name, readWrite: readWrite}
	s.Marshal = s
	return s
}

func (s *fakeSuite) Name() string {
	return s.name
}

func (s *fakeSuite) IsReadWrite() bool {
	return s.readWrite
}

func (s *fakeSuite) RunE() error {
	return nil
}

func TestMarshalerString(t *testing.T) {
	testCases := []struct {
		name      string
		suiteName string
		readWrite bool
		expected  string
	}{
		{
			name:      "read-only suite",
			suiteName: "node-capacity",
			readWrite: false,
			expected:  "node-capacity\tread-only",
		},
		{
			name:      "read-write suite",
			suiteName: "etcd-benchmark",
			readWrite: true,
			expected:  "etcd-benchmark\tread-write",
		},
		{
			name:      "empty suite name",
			suiteName: "",
			readWrite: false,
			expected:  "\tread-only",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := &TestSuiteMarshaler{Marshal: newFakeSuite(tc.suiteName, tc.readWrite)}
			if got := m.String(); got != tc.expected {
				t.Errorf("String() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestMarshalerMarshalJSON(t *testing.T) {
	testCases := []struct {
		name      string
		suiteName string
		readWrite bool
		expected  string
	}{
		{
			name:      "read-only suite",
			suiteName: "node-capacity",
			readWrite: false,
			expected:  `{"name":"node-capacity","readWrite":false}`,
		},
		{
			name:      "read-write suite",
			suiteName: "etcd-benchmark",
			readWrite: true,
			expected:  `{"name":"etcd-benchmark","readWrite":true}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := &TestSuiteMarshaler{Marshal: newFakeSuite(tc.suiteName, tc.readWrite)}

			// MarshalJSON returns an anonymous struct, so assert on the encoded
			// form rather than on the concrete type.
			data, err := m.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON() returned unexpected error: %v", err)
			}
			if string(data) != tc.expected {
				t.Errorf("MarshalJSON() = %s, want %s", data, tc.expected)
			}

			// The output must also be valid JSON that decodes to the same values.
			var decoded struct {
				Name      string `json:"name"`
				ReadWrite bool   `json:"readWrite"`
			}
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("MarshalJSON() produced invalid JSON %s: %v", data, err)
			}
			if decoded.Name != tc.suiteName {
				t.Errorf("decoded name = %q, want %q", decoded.Name, tc.suiteName)
			}
			if decoded.ReadWrite != tc.readWrite {
				t.Errorf("decoded readWrite = %t, want %t", decoded.ReadWrite, tc.readWrite)
			}
		})
	}
}

func TestTestSuiteMarshalerMarshalYAML(t *testing.T) {
	testCases := []struct {
		name      string
		suiteName string
		readWrite bool
	}{
		{
			name:      "read-only suite",
			suiteName: "node-capacity",
			readWrite: false,
		},
		{
			name:      "read-write suite",
			suiteName: "etcd-benchmark",
			readWrite: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := &TestSuiteMarshaler{Marshal: newFakeSuite(tc.suiteName, tc.readWrite)}
			v, err := m.MarshalYAML()
			if err != nil {
				t.Fatalf("MarshalYAML() returned unexpected error: %v", err)
			}

			// MarshalYAML returns an anonymous struct, so assert on the encoded
			// form rather than on the concrete type.
			data, err := yaml.Marshal(v)
			if err != nil {
				t.Fatalf("yaml.Marshal() returned unexpected error: %v", err)
			}

			var decoded struct {
				Name      string `yaml:"name"`
				ReadWrite bool   `yaml:"readWrite"`
			}
			if err := yaml.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("yaml.Unmarshal() returned unexpected error: %v", err)
			}
			if decoded.Name != tc.suiteName {
				t.Errorf("decoded name = %q, want %q", decoded.Name, tc.suiteName)
			}
			if decoded.ReadWrite != tc.readWrite {
				t.Errorf("decoded readWrite = %t, want %t", decoded.ReadWrite, tc.readWrite)
			}
		})
	}
}
