package suites

import (
	"context"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// fakePromAPI satisfies promv1.API via embedding. Tests only check nil/non-nil,
// so no methods need to be implemented.
type fakePromAPI struct{ promv1.API }

var emptyScheme = runtime.NewScheme()

// fakeSuite is a minimal Suite implementation used to exercise marshaling and
// registry logic in isolation from real test suites.
var _ Suite = &fakeSuite{}

type fakeSuite struct {
	SuiteMarshaler
	name          string
	description   string
	exclusive     bool
	optionsSchema Options
}

func newFakeSuite(name, description string, exclusive bool) *fakeSuite {
	s := &fakeSuite{name: name, description: description, exclusive: exclusive}
	s.Marshal = s
	return s
}

func (s *fakeSuite) Name() string        { return s.name }
func (s *fakeSuite) Description() string { return s.description }
func (s *fakeSuite) IsExclusive() bool   { return s.exclusive }
func (s *fakeSuite) RunE(_ context.Context, _, _ string, _ Options) SuiteResult {
	return SuiteResult{}
}
func (s *fakeSuite) OptionsSchema() Options                  { return s.optionsSchema }
func (s *fakeSuite) SetClients(_ *Clients)                   {}
func (s *fakeSuite) SetProgressReporter(_ *ProgressReporter) {}

// fakeOptions stands in for a suite's options struct, such as the
// etcd suite's BenchmarkOptions. Only the primitive field kinds that the
// options structs use are covered.
type fakeOptions struct {
	EtcdEndpoints      string
	EtcdMetricsPort    int
	PutLoadSize        uint64
	JobSuspend         bool
	JobPodReadyTimeout time.Duration
	unexported         string //nolint:unused // asserts that ToSuiteParams skips it
}

// recordingSuite wraps fakeSuite with a SetClients that records what it
// received, so WithClients behaviour can be observed.
var _ Suite = &recordingSuite{}

type recordingSuite struct {
	*fakeSuite

	clients  *Clients
	setCalls int
}

func newRecordingSuite(name string) *recordingSuite {
	return &recordingSuite{fakeSuite: newFakeSuite(name, "fake test suite", false)}
}

func (s *recordingSuite) SetClients(clients *Clients) {
	s.clients = clients
	s.setCalls++
}
func (s *recordingSuite) SetProgressReporter(_ *ProgressReporter) {}
