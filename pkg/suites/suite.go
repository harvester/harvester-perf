package suites

import (
	"context"

	monclient "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	dynclient "k8s.io/client-go/dynamic"
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

// Clients holds the K8s API client sets that are used by the test suites
// to interact with the cluster.
type Clients struct {
	K8sClientSet     k8sclient.Interface
	DynClientSet     dynclient.Interface
	MonClientSet     monclient.Interface
	RestConfig       *rest.Config
	PrometheusClient promv1.API
}

// NewClients creates a new ClientSets with the provided K8s client set.
func NewClients(
	k8sClientSet k8sclient.Interface,
	dynClientSet dynclient.Interface,
	monClientSet monclient.Interface,
	restConfig *rest.Config, prometheusClient promv1.API,
) *Clients {
	return &Clients{
		K8sClientSet:     k8sClientSet,
		DynClientSet:     dynClientSet,
		MonClientSet:     monClientSet,
		RestConfig:       restConfig,
		PrometheusClient: prometheusClient,
	}
}

// WithClients creates a new Suite with the provided Clients.
func WithClients(suite Suite, clients *Clients) Suite {
	suite.SetClients(clients)
	return suite
}
