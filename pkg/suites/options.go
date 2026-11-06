package suites

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"strings"
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

// Canonicalize canonicalizes (standardizes) the Options against a given schema.
// It returns a new Options with the same keys as the original Options, but with
// the values canonicalized against the schema. If a key in the Options does not
// exist in the schema, an error is returned.
//
// Keys are compared case-insensitively against the keys defined in the schema,
// and rewritten to the canonical forms.
func (o Options) Canonicalize(schema Options) (Options, error) {
	canonicalized := Options{}
	canonicalizedKeys, err := o.canonicalizeKeys(schema)
	if err != nil {
		return nil, err
	}

	for k, v := range canonicalizedKeys {
		if nested, isOpt := isOptions(v); isOpt {
			// if a value is a nested option, the corresponding member in the schema must
			// also be a nested option. otherwise, return an error.
			schemaVal, ok := isOptions(schema[k])
			if !ok {
				return nil, fmt.Errorf("option %q is not a nested option in schema", k)
			}

			nestedCanonicalized, err := nested.Canonicalize(schemaVal)
			if err != nil {
				return nil, err
			}
			canonicalized[k] = nestedCanonicalized
			continue
		}

		// if a value is a scalar value, the corresponding member in the schema must
		// also be a scalar value, not a nested option. otherwise, return an error.
		if _, isOpt := isOptions(schema[k]); isOpt {
			return nil, fmt.Errorf("option %q is a nested option in schema but not in options", k)
		}
		canonicalized[k] = v
	}
	return canonicalized, nil
}

func (o Options) canonicalizeKeys(schema Options) (Options, error) {
	// schemaKeys is a map of the lowercased keys to the original keys. This allows
	// for case-insensitive comparison of the keys in the Options against the schema,
	// and then retrieve the original key from the schema to use in the canonicalized
	// Options.
	schemaKeys := map[string]string{}
	for k := range map[string]any(schema) {
		schemaKeys[strings.ToLower(k)] = k
	}

	var (
		canonicalized = Options{}
		errs          error
		reg           = regexp.MustCompile(`[^a-zA-Z0-9]+`)
	)
	for k, v := range o {
		trimmed := strings.TrimSpace(k)
		if trimmed == "" {
			errs = errors.Join(errs, fmt.Errorf("illegal empty option key %q", k))
			continue
		}

		// remove symbols and delimiters (hyphens, underscores)
		safeStr := reg.ReplaceAllString(trimmed, "")

		// error if the canonicalized key does not exist in the schema.
		// if it exists, retrieve the original key from the schema to use in the
		// canonicalized Options.
		canonicalizedKey, exists := schemaKeys[strings.ToLower(safeStr)]
		if !exists {
			errs = errors.Join(errs, fmt.Errorf("option %q not found in options", k))
			continue
		}

		// treat duplicate canonicalized keys as an error
		if _, duplicate := canonicalized[canonicalizedKey]; duplicate {
			errs = errors.Join(errs, fmt.Errorf("duplicate option %q after canonicalization", k))
			continue
		}
		canonicalized[canonicalizedKey] = v
	}
	return canonicalized, errs
}
