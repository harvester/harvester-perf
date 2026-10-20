package suites

import (
	"errors"
	"os"
	"reflect"
	"sort"
	"testing"
)

var (
	nonExclusiveSuite0 = newFakeSuite("test-fake-ro-0", "fake test suite", false)
	nonExclusiveSuite1 = newFakeSuite("test-fake-ro-1", "fake test suite", false)
	exclusiveSuite     = newFakeSuite("test-fake-rw", "fake test suite", true)
)

func TestMain(m *testing.M) {
	// remove any test suites from the registry so that the tests below can run with
	// fake test suites
	r.mu.Lock()
	r.suites = map[string]Suite{}
	r.mu.Unlock()

	Register(nonExclusiveSuite0)
	Register(nonExclusiveSuite1)
	Register(exclusiveSuite)

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
	if !reflect.DeepEqual(every, []Suite{nonExclusiveSuite0, nonExclusiveSuite1, exclusiveSuite}) {
		t.Errorf("All(true) = %v, want [%v, %v, %v]", every, nonExclusiveSuite0.Name(), nonExclusiveSuite1.Name(), exclusiveSuite.Name())
	}
}

// TestAllSorted checks that All returns suites sorted by name, not
// registration order or map iteration order.
func TestAllSorted(t *testing.T) {
	got := All()
	want := []string{nonExclusiveSuite0.Name(), nonExclusiveSuite1.Name(), exclusiveSuite.Name()}
	sort.Strings(want)

	var names []string
	for _, s := range got {
		names = append(names, s.Name())
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("All() names = %v, want %v", names, want)
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
			name:     "one non-exclusive suite",
			names:    []string{nonExclusiveSuite0.Name()},
			expected: []Suite{nonExclusiveSuite0},
		},
		{
			name:     "multiple non-exclusive suites",
			names:    []string{nonExclusiveSuite0.Name(), nonExclusiveSuite1.Name()},
			expected: []Suite{nonExclusiveSuite0, nonExclusiveSuite1},
		},
		{
			name:     "one exclusive suite",
			names:    []string{exclusiveSuite.Name()},
			expected: []Suite{exclusiveSuite},
		},
		{
			name:     "repeated name yields repeated results",
			names:    []string{nonExclusiveSuite0.Name(), nonExclusiveSuite0.Name(), nonExclusiveSuite1.Name()},
			expected: []Suite{nonExclusiveSuite0, nonExclusiveSuite1},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Find(tc.names)
			if err != nil {
				t.Fatalf("Find() returned error: %v", err)
			}
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

	// test non-exclusive and exclusive suites together, which should return an error
	if _, err := Find([]string{nonExclusiveSuite0.Name(), exclusiveSuite.Name()}); !errors.Is(err, ErrSuiteExclusiveConflict) {
		t.Errorf("Find() returned mismatched error, want %q error for multiple exclusive suites", ErrSuiteExclusiveConflict)
	}

	if _, err := Find([]string{nonExclusiveSuite0.Name(), nonExclusiveSuite1.Name(), exclusiveSuite.Name()}); !errors.Is(err, ErrSuiteExclusiveConflict) {
		t.Errorf("Find() returned mismatched error, want %q error for multiple exclusive suites", ErrSuiteExclusiveConflict)
	}
}

func TestFindMissingSuite(t *testing.T) {
	suites := []string{"non-existent-suite"}
	if _, err := Find(suites); !errors.Is(err, ErrSuiteNotFound) {
		t.Fatalf("Find() returned mismatched error, want %q error for missing suite", ErrSuiteNotFound)
	}
}
