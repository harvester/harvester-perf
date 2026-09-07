package suites

import (
	"testing"

	monclient "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
	monfake "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned/fake"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	dynclient "k8s.io/client-go/dynamic"
	dynfake "k8s.io/client-go/dynamic/fake"
	k8sclient "k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func TestNewClients(t *testing.T) {
	var (
		k8sClientSet = k8sfake.NewClientset()
		dynClientSet = dynfake.NewSimpleDynamicClient(emptyScheme)
		monClientSet = monfake.NewSimpleClientset()
		restConfig   = &rest.Config{Host: "https://harvester.example.com:6443"}
		promClient   = fakePromAPI{}
	)

	testCases := []struct {
		name         string
		k8sClientSet k8sclient.Interface
		dynClientSet dynclient.Interface
		monClientSet monclient.Interface
		promClient   promv1.API
		restConfig   *rest.Config
		wantPromNil  bool
	}{
		{
			name:         "all clients set",
			k8sClientSet: k8sClientSet,
			dynClientSet: dynClientSet,
			monClientSet: monClientSet,
			promClient:   promClient,
			restConfig:   restConfig,
		},
		{
			name:         "nil rest config",
			k8sClientSet: k8sClientSet,
			dynClientSet: dynClientSet,
			monClientSet: monClientSet,
			promClient:   promClient,
			restConfig:   nil,
		},
		{
			name:         "nil prom client",
			k8sClientSet: k8sClientSet,
			dynClientSet: dynClientSet,
			monClientSet: monClientSet,
			promClient:   nil,
			restConfig:   restConfig,
			wantPromNil:  true,
		},
		{
			name:         "nil k8s clients",
			k8sClientSet: nil,
			dynClientSet: nil,
			monClientSet: nil,
			promClient:   promClient,
			restConfig:   restConfig,
		},
		{
			name:        "all nil",
			promClient:  nil,
			restConfig:  nil,
			wantPromNil: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClients(tc.k8sClientSet, tc.dynClientSet, tc.monClientSet, tc.promClient, tc.restConfig)
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
			if tc.wantPromNil && c.PromClient != nil {
				t.Errorf("PromClient = %v, want nil", c.PromClient)
			}
			if !tc.wantPromNil && c.PromClient == nil {
				t.Errorf("PromClient = nil, want non-nil")
			}
			if c.RestConfig != tc.restConfig {
				t.Errorf("RestConfig = %v, want %v", c.RestConfig, tc.restConfig)
			}
		})
	}
}

func TestWithClients(t *testing.T) {
	s := newRecordingSuite("test-fake-with-clients")
	clients := NewClients(
		k8sfake.NewClientset(),
		dynfake.NewSimpleDynamicClient(emptyScheme),
		monfake.NewSimpleClientset(),
		fakePromAPI{},
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

func TestWithClients_PromClientForwarded(t *testing.T) {
	clients := NewClients(
		k8sfake.NewClientset(),
		dynfake.NewSimpleDynamicClient(emptyScheme),
		monfake.NewSimpleClientset(),
		fakePromAPI{},
		&rest.Config{Host: "https://harvester.example.com:6443"},
	)

	s := newRecordingSuite("test-prom-client-forwarded")
	WithClients(s, clients)

	if s.clients == nil {
		t.Fatal("SetClients was not called")
	}
	if s.clients.PromClient == nil {
		t.Errorf("PromClient in forwarded clients = nil, want non-nil")
	}
}
