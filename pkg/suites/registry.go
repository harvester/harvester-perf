package suites

import (
	"slices"
	"sort"
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
	suites map[string]Suite
}

// All returns all registered test suites, sorted by suite name.
func All() []Suite {
	var (
		sortedKey []string
		suites    []Suite
	)
	for _, s := range r.suites {
		sortedKey = append(sortedKey, s.Name())
	}
	sort.Strings(sortedKey)

	for _, suite := range sortedKey {
		suites = append(suites, r.suites[suite])
	}
	return suites
}

// Find returns the test suites that match the provided names. If a name does not
// match any registered test suite, it is ignored.
func Find(names []string) []Suite {
	suites := []Suite{}
	for _, name := range names {
		if _, exists := r.suites[name]; exists {
			if !slices.Contains(suites, r.suites[name]) {
				suites = append(suites, r.suites[name])
			}
		}
	}
	return suites
}

func Register(suite Suite) {
	once.Do(func() {
		r.suites = map[string]Suite{}
	})

	r.mu.Lock()
	defer r.mu.Unlock()
	r.suites[suite.Name()] = suite
}
