package suites

import (
	"context"

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

	name        string
	description string
	readWrite   bool
}

func newFakeSuite(name, description string, readWrite bool) *fakeSuite {
	s := &fakeSuite{name: name, description: description, readWrite: readWrite}
	s.Marshal = s
	return s
}

func (s *fakeSuite) Name() string        { return s.name }
func (s *fakeSuite) Description() string { return s.description }
func (s *fakeSuite) IsReadWrite() bool   { return s.readWrite }
func (s *fakeSuite) RunE(_ context.Context, _, _ string, _ Options) (SuiteResult, error) {
	return SuiteResult{}, nil
}
func (s *fakeSuite) SetClients(_ *Clients)                   {}
func (s *fakeSuite) SetProgressReporter(_ *ProgressReporter) {}

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
