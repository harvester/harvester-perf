package prometheus

import (
	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"k8s.io/client-go/rest"
)

const prometheusSVCProxy = "/api/v1/namespaces/cattle-monitoring-system/services/rancher-monitoring-prometheus:9090/proxy"

// New creates a Prometheus HTTP API client. An empty server URL uses the
// Prometheus service proxy on the Kubernetes API server from restConfig.
//
// The client uses the transport derived from restConfig for authentication
// and TLS configuration.
func New(serverURL string, restConfig *rest.Config) (promv1.API, error) {
	if serverURL == "" {
		serverURL = restConfig.Host + prometheusSVCProxy
	}

	transport, err := rest.TransportFor(restConfig)
	if err != nil {
		return nil, err
	}
	client, err := promapi.NewClient(promapi.Config{
		Address:      serverURL,
		RoundTripper: transport,
	})
	if err != nil {
		return nil, err
	}
	return promv1.NewAPI(client), nil
}
