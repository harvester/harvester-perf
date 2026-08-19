package suites

import (
	"slices"
	"sync"
)

var (
	// r is the singleton instance of the Registry that holds all registered test
	// suites.
	r    Registry
	once sync.Once
)

// Registry is a thread-safe registry for managing test suites. Every test suite
// is registered in this registry.
type Registry struct {
	mu     sync.Mutex
	suites map[string]TestSuite
}

// All returns all registered test suites. If includeRW is true, it returns all
// read-write and read-only test suites. Otherwise, it returns only read-only
// test suites.
func All(includeReadWrite bool) []TestSuite {
	suites := []TestSuite{}
	for _, s := range r.suites {
		if includeReadWrite || !s.IsReadWrite() {
			suites = append(suites, s)
		}
	}
	return suites
}

// Find returns the test suites that match the provided names. If a name does not
// match any registered test suite, it is ignored.
func Find(names []string) []TestSuite {
	suites := []TestSuite{}
	for _, name := range names {
		if _, exists := r.suites[name]; exists {
			if !slices.Contains(suites, r.suites[name]) {
				suites = append(suites, r.suites[name])
			}
		}
	}
	return suites
}

func register(suite TestSuite) {
	once.Do(func() {
		r.suites = map[string]TestSuite{}
	})

	r.mu.Lock()
	defer r.mu.Unlock()
	r.suites[suite.Name()] = suite
}
