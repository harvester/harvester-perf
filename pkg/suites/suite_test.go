package suites

import (
	"testing"

	monclient "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
	monfake "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned/fake"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynclient "k8s.io/client-go/dynamic"
	dynfake "k8s.io/client-go/dynamic/fake"
	k8sclient "k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

var _ Suite = &recordingSuite{}

// fakeProm satisfies promv1.API without implementing any methods. Calling any
// method on it panics, which is fine — these tests only check that the value
// is stored, never that it is called.
type fakeProm struct{ promv1.API }

var emptyScheme = runtime.NewScheme()

// recordingSuite wraps fakeSuite with a SetClients implementation that records
// what it was handed, so that WithClients can be observed.
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

func TestNewClients(t *testing.T) {
	var (
		k8sClientSet = k8sfake.NewClientset()
		dynClientSet = dynfake.NewSimpleDynamicClient(emptyScheme)
		monClientSet = monfake.NewSimpleClientset()
		restConfig   = &rest.Config{Host: "https://harvester.example.com:6443"}
		promClient   = &fakeProm{}
	)

	testCases := []struct {
		name         string
		k8sClientSet k8sclient.Interface
		dynClientSet dynclient.Interface
		monClientSet monclient.Interface
		restConfig   *rest.Config
		promClient   promv1.API
	}{
		{
			name:         "all fields set",
			k8sClientSet: k8sClientSet,
			dynClientSet: dynClientSet,
			monClientSet: monClientSet,
			restConfig:   restConfig,
			promClient:   promClient,
		},
		{
			name:         "nil rest config",
			k8sClientSet: k8sClientSet,
			dynClientSet: dynClientSet,
			monClientSet: monClientSet,
			restConfig:   nil,
		},
		{
			name:         "nil client set",
			k8sClientSet: nil,
			dynClientSet: nil,
			monClientSet: nil,
			restConfig:   restConfig,
		},
		{
			name:         "all nil",
			k8sClientSet: nil,
			dynClientSet: nil,
			monClientSet: nil,
			restConfig:   nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClients(tc.k8sClientSet, tc.dynClientSet, tc.monClientSet, tc.restConfig, tc.promClient)
			if c == nil {
				t.Fatal("NewClients() returned nil, want non-nil *Clients")
			}
			if c.K8sClientSet != tc.k8sClientSet {
				t.Errorf("K8sClientSet = %v, want %v", c.K8sClientSet, tc.k8sClientSet)
			}
			if c.DynClientSet != tc.dynClientSet {
				t.Errorf("DynClientSet = %v, want %v", c.DynClientSet, tc.dynClientSet)
			}
			if c.MonClientSet != tc.monClientSet {
				t.Errorf("MonClientSet = %v, want %v", c.MonClientSet, tc.monClientSet)
			}
			if c.RestConfig != tc.restConfig {
				t.Errorf("RestConfig = %v, want %v", c.RestConfig, tc.restConfig)
			}
			if c.PrometheusClient != tc.promClient {
				t.Errorf("PrometheusClient = %v, want %v", c.PrometheusClient, tc.promClient)
			}
		})
	}
}

// TestWithClients checks that WithClients hands the clients to the suite and
// returns the same suite instance.
func TestWithClients(t *testing.T) {
	var (
		clients1 = NewClients(k8sfake.NewClientset(), nil, nil, &rest.Config{Host: "https://harvester.example.com:6443"}, nil)
		clients2 = NewClients(k8sfake.NewClientset(), nil, nil, &rest.Config{Host: "https://harvester2.example.com:6443"}, &fakeProm{})
	)

	testCases := []struct {
		name            string
		clients         []*Clients
		wantSetCalls    int
		wantLastClients *Clients
	}{
		{
			name:            "single call",
			clients:         []*Clients{clients1},
			wantSetCalls:    1,
			wantLastClients: clients1,
		},
		{
			name:            "second call overwrites first",
			clients:         []*Clients{clients1, clients2},
			wantSetCalls:    2,
			wantLastClients: clients2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s := newRecordingSuite("test-fake-with-clients")
			var got Suite
			for _, c := range tc.clients {
				got = WithClients(s, c)
			}
			if got != Suite(s) {
				t.Errorf("WithClients() = %v, want the same suite instance %v", got, s)
			}
			if s.setCalls != tc.wantSetCalls {
				t.Errorf("SetClients called %d times, want %d", s.setCalls, tc.wantSetCalls)
			}
			if s.clients != tc.wantLastClients {
				t.Errorf("SetClients received %v, want %v", s.clients, tc.wantLastClients)
			}
		})
	}
}
