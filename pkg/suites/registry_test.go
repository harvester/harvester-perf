package suites

import (
	"os"
	"reflect"
	"sort"
	"testing"
)

var (
	readOnlySuite0 = newFakeSuite("test-fake-ro-0", "fake test suite", false)
	readOnlySuite1 = newFakeSuite("test-fake-ro-1", "fake test suite", false)
	readWriteSuite = newFakeSuite("test-fake-rw", "fake test suite", true)
)

func TestMain(m *testing.M) {
	// remove any test suites from the registry so that the tests below can run with
	// fake test suites
	r.mu.Lock()
	r.suites = map[string]Suite{}
	r.mu.Unlock()

	Register(readOnlySuite0)
	Register(readOnlySuite1)
	Register(readWriteSuite)

	code := m.Run()
	os.Exit(code)
}

// TestAll checks that All returns only read-only and read-write test suites
// based on the parameter passed.
func TestAll(t *testing.T) {
	every := All()
	sort.Slice(every, func(i, j int) bool {
		return every[i].Name() < every[j].Name()
	})
	if !reflect.DeepEqual(every, []Suite{readOnlySuite0, readOnlySuite1, readWriteSuite}) {
		t.Errorf("All(true) = %v, want [%v, %v, %v]", every, readOnlySuite0.Name(), readOnlySuite1.Name(), readWriteSuite.Name())
	}
}

func TestFind(t *testing.T) {
	testCases := []struct {
		name     string
		names    []string
		expected []Suite
	}{
		{
			name:     "nil names returns no suites",
			names:    nil,
			expected: []Suite{},
		},
		{
			name:     "empty names returns no suites",
			names:    []string{},
			expected: []Suite{},
		},
		{
			name:     "single match",
			names:    []string{readOnlySuite0.Name()},
			expected: []Suite{readOnlySuite0},
		},
		{
			name:     "multiple matches",
			names:    []string{readOnlySuite0.Name(), readOnlySuite1.Name(), readWriteSuite.Name()},
			expected: []Suite{readOnlySuite0, readOnlySuite1, readWriteSuite},
		},
		{
			name:     "repeated name yields repeated results",
			names:    []string{readOnlySuite0.Name(), readOnlySuite0.Name(), readOnlySuite1.Name()},
			expected: []Suite{readOnlySuite0, readOnlySuite1},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := Find(tc.names)
			if got == nil {
				t.Fatal("Find() returned nil, want non-nil slice")
			}

			sort.Slice(got, func(i, j int) bool {
				return got[i].Name() < got[j].Name()
			})
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("Find(%v) = %v, want %v", tc.names, got, tc.expected)
			}
		})
	}
}
