package suites

import (
	"maps"
)

const DefaultNamespace = "harvester-perf-system"

// Options contains custom options for test suites. The keys are the names of the
// test suites, and the values are the options for each suite.
type Options map[string]any

// Merge merges the current Options with another Options, returning a new Options.
// If a key exists in both Options, the value from the other Options will
// overwrite the value in the current Options. Nested Options will be merged
// recursively.
func (o Options) Merge(other Options) Options {
	merged := Options{}
	maps.Copy(merged, o)

	// iterate over the other Options to identify nested Options. if the same nested
	// Options also exists in merged, merge them recursively with the other Options
	// taking precedence. otherwise, just copy the value to merged.
	for k, v := range other {
		src, srcIsOptions := isOptions(v)
		dst, dstIsOptions := isOptions(merged[k])
		if srcIsOptions && dstIsOptions {
			merged[k] = dst.Merge(src)
			continue
		}
		merged[k] = v
	}
	return merged
}

func isOptions(v any) (Options, bool) {
	switch m := v.(type) {
	case Options:
		return m, true
	case map[string]any:
		return m, true
	}
	return nil, false
}
