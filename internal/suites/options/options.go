package options

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"strings"
	"time"

	"github.com/harvester/hvperf/pkg/suites"
)

// FromOptions converts the given suites.Options to the specific options type V.
func FromOptions[V any](opts suites.Options) (V, error) {
	var o V
	opts, err := parseDurations(reflect.TypeFor[V](), opts)
	if err != nil {
		return o, err
	}

	b, err := json.Marshal(opts)
	if err != nil {
		return o, err
	}

	if err := json.Unmarshal(b, &o); err != nil {
		return o, err
	}
	return o, nil
}

// compare opts with type t to identify duration fields. t is usually options
// types like KBenchOptions, BenchmarkOptions etc. if found, parse and convert
// the string to time.Duration type.
func parseDurations(t reflect.Type, opts suites.Options) (suites.Options, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return opts, nil
	}

	out := maps.Clone(opts)
	for k, v := range opts {
		s, ok := v.(string)
		if !ok {
			continue
		}

		// see if k (which belongs to opts) is a time.Duration field in t
		f, ok := structField(t, k)
		if !ok || f.Type != reflect.TypeFor[time.Duration]() {
			continue
		}
		d, err := time.ParseDuration(s)
		if err != nil {
			return nil, fmt.Errorf("option %s: %w", k, err)
		}
		out[k] = d
	}
	return out, nil
}

// find field in the struct type t that matches the given name, case-insensitively.
func structField(t reflect.Type, field string) (reflect.StructField, bool) {
	return t.FieldByNameFunc(func(n string) bool {
		return strings.EqualFold(n, field)
	})
}
