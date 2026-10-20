package suites

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"sync"
)

var (
	// r is the singleton instance of the Registry that holds all registered test
	// suites.
	r    Registry
	once sync.Once

	ErrSuiteExclusiveConflict = errors.New("multiple exclusive test suites found - only one exclusive test suite per run")
	ErrSuiteNotFound          = errors.New("test suite not found")
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
// match any registered test suite, it is returned as an error.
func Find(names []string) ([]Suite, error) {
	var (
		suites         = []Suite{}
		exclusiveCount int
		err            error
	)

	for _, name := range names {
		if s, exists := r.suites[name]; exists {
			if !slices.Contains(suites, s) {
				suites = append(suites, s)
				if s.IsExclusive() {
					exclusiveCount++
				}
			}
			continue
		}
		err = errors.Join(err, fmt.Errorf("%w: %q", ErrSuiteNotFound, name))
	}
	if err != nil {
		return nil, err
	}

	if len(suites) > 1 && exclusiveCount > 0 {
		return nil, ErrSuiteExclusiveConflict
	}
	return suites, nil
}

func Register(suite Suite) {
	once.Do(func() {
		r.suites = map[string]Suite{}
	})

	r.mu.Lock()
	defer r.mu.Unlock()
	r.suites[suite.Name()] = suite
}
