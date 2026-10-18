package suites

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestOptionsMerge(t *testing.T) {
	testCases := []struct {
		name     string
		o        Options
		other    Options
		expected Options
	}{
		{
			name:     "both empty",
			o:        Options{},
			other:    Options{},
			expected: Options{},
		},
		{
			name:     "other empty keeps original values",
			o:        Options{"a": 1},
			other:    Options{},
			expected: Options{"a": 1},
		},
		{
			name:     "original empty takes other values",
			o:        Options{},
			other:    Options{"a": 1},
			expected: Options{"a": 1},
		},
		{
			name:     "disjoint keys union",
			o:        Options{"a": 1},
			other:    Options{"b": 2},
			expected: Options{"a": 1, "b": 2},
		},
		{
			name:     "overlapping key: other wins",
			o:        Options{"a": 1},
			other:    Options{"a": 2},
			expected: Options{"a": 2},
		},
		{
			name:     "nil other leaves original unchanged",
			o:        Options{"a": 1},
			other:    nil,
			expected: Options{"a": 1},
		},
		{
			name:     "nil receiver with non-empty other",
			o:        nil,
			other:    Options{"a": 1},
			expected: Options{"a": 1},
		},
		{
			name:     "nested options merged recursively",
			o:        Options{"a": Options{"x": 1, "y": 2}},
			other:    Options{"a": Options{"y": 3, "z": 4}},
			expected: Options{"a": Options{"x": 1, "y": 3, "z": 4}},
		},
		{
			name:     "nested map[string]any is treated as Options",
			o:        Options{"a": map[string]any{"x": 1, "y": 2}},
			other:    Options{"a": map[string]any{"y": 3}},
			expected: Options{"a": Options{"x": 1, "y": 3}},
		},
		{
			name:     "mixed Options and map[string]any merge",
			o:        Options{"a": Options{"x": 1}},
			other:    Options{"a": map[string]any{"y": 2}},
			expected: Options{"a": Options{"x": 1, "y": 2}},
		},
		{
			name:     "deeply nested options merged recursively",
			o:        Options{"a": Options{"b": Options{"x": 1, "y": 2}}},
			other:    Options{"a": Options{"b": Options{"y": 3}}},
			expected: Options{"a": Options{"b": Options{"x": 1, "y": 3}}},
		},
		{
			name:     "nested options only in other are added",
			o:        Options{"a": 1},
			other:    Options{"b": Options{"x": 1}},
			expected: Options{"a": 1, "b": Options{"x": 1}},
		},
		{
			name:     "scalar in other replaces nested options",
			o:        Options{"a": Options{"x": 1}},
			other:    Options{"a": 2},
			expected: Options{"a": 2},
		},
		{
			name:     "nested options in other replace scalar",
			o:        Options{"a": 1},
			other:    Options{"a": Options{"x": 1}},
			expected: Options{"a": Options{"x": 1}},
		},
		{
			name:     "non-Options map is replaced, not merged",
			o:        Options{"a": map[string]string{"x": "1", "y": "2"}},
			other:    Options{"a": map[string]string{"y": "3"}},
			expected: Options{"a": map[string]string{"y": "3"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			merged := tc.o.Merge(tc.other)

			if diff := cmp.Diff(merged, tc.expected); diff != "" {
				t.Errorf("OptionsMerge() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestOptionsMergeDoesNotMutateReceiver(t *testing.T) {
	o := Options{"a": 1}
	other := Options{"a": 2, "b": 3}

	o.Merge(other)

	want := Options{"a": 1}
	if diff := cmp.Diff(o, want); diff != "" {
		t.Errorf("OptionsMerge() mismatch (-want +got):\n%s", diff)
	}
}

func TestOptionsMergeDoesNotMutateNestedReceiver(t *testing.T) {
	o := Options{"a": Options{"x": 1}}
	other := Options{"a": Options{"x": 2, "y": 3}}

	merged := o.Merge(other)
	merged["a"].(Options)["z"] = 4

	if diff := cmp.Diff(o, Options{"a": Options{"x": 1}}); diff != "" {
		t.Errorf("receiver mutated (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(other, Options{"a": Options{"x": 2, "y": 3}}); diff != "" {
		t.Errorf("argument mutated (-want +got):\n%s", diff)
	}
}
