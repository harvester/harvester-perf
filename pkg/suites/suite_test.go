package suites

import (
	"testing"

	monclient "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
	monfake "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned/fake"
	"k8s.io/apimachinery/pkg/runtime"
	dynclient "k8s.io/client-go/dynamic"
	dynfake "k8s.io/client-go/dynamic/fake"
	k8sclient "k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

var _ Suite = &recordingSuite{}

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
	)

	testCases := []struct {
		name         string
		k8sClientSet k8sclient.Interface
		dynClientSet dynclient.Interface
		monClientSet monclient.Interface
		restConfig   *rest.Config
	}{
		{
			name:         "clients set and rest config",
			k8sClientSet: k8sClientSet,
			dynClientSet: dynClientSet,
			monClientSet: monClientSet,
			restConfig:   restConfig,
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
			c := NewClients(tc.k8sClientSet, tc.dynClientSet, tc.monClientSet, tc.restConfig)
			if c == nil {
				t.Fatal("NewClients() returned nil, want non-nil *Clients")
			}
			if c.K8sClientSet != tc.k8sClientSet {
				t.Errorf("K8sClientSet = %v, want %v", c.K8sClientSet, tc.k8sClientSet)
			}
			if c.DynClientSet != tc.dynClientSet {
				t.Errorf("K8sClientSet = %v, want %v", c.DynClientSet, tc.dynClientSet)
			}
			if c.RestConfig != tc.restConfig {
				t.Errorf("RestConfig = %v, want %v", c.RestConfig, tc.restConfig)
			}
		})
	}
}

// TestWithClients checks that WithClients hands the clients to the suite and
// returns the same suite instance.
func TestWithClients(t *testing.T) {
	s := newRecordingSuite("test-fake-with-clients")
	clients := NewClients(
		k8sfake.NewClientset(),
		dynfake.NewSimpleDynamicClient(emptyScheme),
		monfake.NewSimpleClientset(),
		&rest.Config{Host: "https://harvester.example.com:6443"},
	)

	got := WithClients(s, clients)

	if got != Suite(s) {
		t.Errorf("WithClients() = %v, want the same suite instance %v", got, s)
	}
	if s.setCalls != 1 {
		t.Errorf("SetClients called %d times, want 1", s.setCalls)
	}
	if s.clients != clients {
		t.Errorf("SetClients received %v, want %v", s.clients, clients)
	}
}
