package suites

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

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

func TestCanonicalize(t *testing.T) {
	s := fakeSuite{
		name: "fake-suite",
		optionsSchema: Options{
			"CPURequest":          "200m",
			"EtcdEndpoints":       "https://localhost:2379",
			"PutLoadSize":         10000,
			"JobSuspend":          false,
			"JobPodReadyTimeout":  180 * time.Second,
			"PVCStorageClassName": "longhorn",
		},
	}

	t.Run("overrides canonicalized with no errors", func(t *testing.T) {
		overrides := Options{
			"cpuRequest":          "200m",
			"etcdEndpoints":       "https://etcd.cluster:2379",
			"putloadsize":         20000,
			"jobsuspend":          true,
			"jobPodReadyTimeout":  1800 * time.Second,
			"pvcStorageClassName": "custom-storage-class",
		}

		got, err := overrides.Canonicalize(s.OptionsSchema())
		if err != nil {
			t.Fatalf("Canonicalize() returned an error: %v", err)
		}

		want := Options{
			"CPURequest":          "200m",
			"EtcdEndpoints":       "https://etcd.cluster:2379",
			"PutLoadSize":         20000,
			"JobSuspend":          true,
			"JobPodReadyTimeout":  1800 * time.Second,
			"PVCStorageClassName": "custom-storage-class",
		}

		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("Canonicalize() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("unidentified field fails canonicalization", func(t *testing.T) {
		overrides := Options{
			"foobarCPU": "200m",
		}

		if _, err := overrides.Canonicalize(s.OptionsSchema()); err == nil {
			t.Fatalf("Canonicalize() did not return an error for unidentified field")
		}
	})
}

func TestCanonicalizeKeys(t *testing.T) {
	testCases := []struct {
		name string
		o    Options
		want Options
	}{
		{
			name: "lowercase key",
			o:    Options{"foobar": struct{}{}},
			want: Options{"FooBar": struct{}{}},
		},
		{
			name: "camelCase key",
			o:    Options{"fooBar": struct{}{}},
			want: Options{"FooBar": struct{}{}},
		},
		{
			name: "multiple upper cases",
			o:    Options{"fooBAR": struct{}{}},
			want: Options{"FooBar": struct{}{}},
		},
		{
			name: "key with hyphen",
			o:    Options{"foo-bar": struct{}{}},
			want: Options{"FooBar": struct{}{}},
		},
		{
			name: "key with underscore",
			o:    Options{"foo_bar": struct{}{}},
			want: Options{"FooBar": struct{}{}},
		},
		{
			name: "key with period",
			o:    Options{"foo.bar": struct{}{}},
			want: Options{"FooBar": struct{}{}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			schema := Options{
				"FooBar": struct{}{},
			}
			got, err := tc.o.canonicalizeKeys(schema)
			if err != nil {
				t.Fatalf("canonicalizeKeys() returned an error: %v", err)
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("canonicalizeKeys() mismatch (-want +got):\n%s", diff)
			}
		})
	}

	t.Run("multiple keys with different delimiters", func(t *testing.T) {
		schema := Options{
			"FooBar0": struct{}{},
			"FooBar1": struct{}{},
			"FooBar2": struct{}{},
			"FooBar3": struct{}{},
			"FooBar4": struct{}{},
		}
		o := Options{"foo.bar0": "", "foo-bar1": "", "foo_bar2": "", "fooBar3": "", "foobar4": ""}
		got, err := o.canonicalizeKeys(schema)
		if err != nil {
			t.Fatalf("canonicalizeKeys() returned an error: %v", err)
		}
		gotKeys := slices.Sorted(maps.Keys(got))

		want := Options{"FooBar0": "", "FooBar1": "", "FooBar2": "", "FooBar3": "", "FooBar4": ""}
		wantKeys := slices.Sorted(maps.Keys(want))

		if diff := cmp.Diff(wantKeys, gotKeys); diff != "" {
			t.Errorf("canonicalizeKeys() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("can't catch typos in keys", func(t *testing.T) {
		schema := Options{
			"FooBar": struct{}{},
		}
		o := Options{"fooBarr": ""}
		got, err := o.canonicalizeKeys(schema)
		if err == nil {
			t.Errorf("canonicalizeKeys() should return an error for typo keys, but got: %v", got)
		}
	})
}

func TestCanonicalizeErrors(t *testing.T) {
	schema := Options{
		"FooBar": 1,
		"Nested": Options{"Inner": 1},
	}

	testCases := []struct {
		name    string
		o       Options
		wantErr string
	}{
		{
			name:    "empty key",
			o:       Options{"": 1},
			wantErr: "empty option key",
		},
		{
			name:    "whitespace-only key",
			o:       Options{"  ": 1},
			wantErr: "empty option key",
		},
		{
			name:    "duplicate keys after canonicalization",
			o:       Options{"fooBar": 1, "foo_bar": 2},
			wantErr: "duplicate option",
		},
		{
			name:    "nested value where schema is scalar",
			o:       Options{"fooBar": Options{"x": 1}},
			wantErr: "not a nested option in schema",
		},
		{
			name:    "scalar value where schema is nested",
			o:       Options{"nested": 1},
			wantErr: "is a nested option in schema",
		},
		{
			name:    "unknown key inside nested option",
			o:       Options{"nested": Options{"nope": 1}},
			wantErr: `option "nope" not found`,
		},
		{
			name:    "multiple unknown keys are all reported",
			o:       Options{"nope1": 1, "nope2": 2},
			wantErr: "nope1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.o.Canonicalize(schema)
			if err == nil {
				t.Fatalf("Canonicalize() returned no error, want one containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Canonicalize() error = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestCanonicalizeNested(t *testing.T) {
	schema := Options{"Nested": Options{"InnerKey": 1}}
	// nested options decoded from YAML arrive as map[string]any, not Options
	o := Options{"nested": map[string]any{"inner-key": 2}}

	got, err := o.Canonicalize(schema)
	if err != nil {
		t.Fatalf("Canonicalize() returned an error: %v", err)
	}
	want := Options{"Nested": Options{"InnerKey": 2}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Canonicalize() mismatch (-want +got):\n%s", diff)
	}
}

func TestCanonicalizeNilReceiver(t *testing.T) {
	var o Options
	got, err := o.Canonicalize(Options{"FooBar": 1})
	if err != nil {
		t.Fatalf("Canonicalize() returned an error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Canonicalize() = %v, want empty", got)
	}
}
