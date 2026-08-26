package suites

import (
	"context"

	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Suite defines the interface that every test suite must implement.
type Suite interface {
	Name() string
	Description() string
	IsReadWrite() bool
	RunE(ctx context.Context, runID, namespace string, opt Options) (SuiteResult, error)
	SetClients(clients *Clients)
}

// Options contains custom options for test suites. The keys are the names of the
// test suites, and the values are the options for each suite.
type Options map[string]any

// Clients holds the K8s API client sets that are used by the test suites
// to interact with the cluster.
type Clients struct {
	K8sClientSet k8sclient.Interface
	RestConfig   *rest.Config
}

// NewClients creates a new ClientSets with the provided K8s client set.
func NewClients(k8sClientSet k8sclient.Interface, restConfig *rest.Config) *Clients {
	return &Clients{
		K8sClientSet: k8sClientSet,
		RestConfig:   restConfig,
	}
}

// WithClients creates a new Suite with the provided Clients.
func WithClients(suite Suite, clients *Clients) Suite {
	suite.SetClients(clients)
	return suite
}
