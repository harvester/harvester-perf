package options_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/harvester/hvperf/internal/suites/density"
	"github.com/harvester/hvperf/internal/suites/etcd"
	"github.com/harvester/hvperf/internal/suites/network"
	"github.com/harvester/hvperf/internal/suites/nodes"
	"github.com/harvester/hvperf/internal/suites/options"
	"github.com/harvester/hvperf/internal/suites/storage"
	"github.com/harvester/hvperf/pkg/suites"
)

// TestSchemaKeysMapToStructFields checks that every key in each Default*Options
// schema, including nested sections, names a field of the suite's options
// struct. A key with no matching field is silently dropped by FromOptions, so
// a typo in a default would otherwise go unnoticed.
func TestSchemaKeysMapToStructFields(t *testing.T) {
	testCases := []struct {
		name   string
		schema suites.Options
		typ    reflect.Type
	}{
		{"density", options.DefaultVMDensityOptions(), reflect.TypeFor[density.Options]()},
		{"etcd-benchmark", options.DefaultEtcdBenchmarkOptions(), reflect.TypeFor[etcd.BenchmarkOptions]()},
		{"network-benchmark", options.DefaultNetworkOptions(), reflect.TypeFor[network.NetworkOptions]()},
		{"node-capacity", options.DefaultNodeCapacityOptions(), reflect.TypeFor[nodes.CapacityOptions]()},
		{"storage-kbench", options.DefaultKBenchOptions(), reflect.TypeFor[storage.KBenchOptions]()},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assertKeysMapToFields(t, tc.schema, tc.typ, "")
		})
	}
}

func assertKeysMapToFields(t *testing.T, schema suites.Options, typ reflect.Type, path string) {
	t.Helper()
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		t.Errorf("%s: want struct type, got %s", path, typ)
		return
	}

	for k, v := range schema {
		field, ok := typ.FieldByNameFunc(func(n string) bool {
			return strings.EqualFold(n, k)
		})
		if !ok {
			t.Errorf("schema key %q has no matching field in %s", path+k, typ)
			continue
		}
		if nested, isNested := v.(suites.Options); isNested {
			assertKeysMapToFields(t, nested, field.Type, path+k+".")
		}
	}
}
