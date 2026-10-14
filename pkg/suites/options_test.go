package suites

import (
	"reflect"
	"testing"
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			merged := tc.o.Merge(tc.other)

			if !reflect.DeepEqual(*merged, tc.expected) {
				t.Errorf("Merge() = %v, want %v", *merged, tc.expected)
			}
		})
	}
}

func TestOptionsMergeDoesNotMutateReceiver(t *testing.T) {
	o := Options{"a": 1}
	other := Options{"a": 2, "b": 3}

	o.Merge(other)

	want := Options{"a": 1}
	if !reflect.DeepEqual(o, want) {
		t.Errorf("Merge() mutated receiver: got %v, want %v", o, want)
	}
}
