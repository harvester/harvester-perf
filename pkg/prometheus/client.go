package prometheus

import (
	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"k8s.io/client-go/rest"
)

const prometheusSVCProxy = "/api/v1/namespaces/cattle-monitoring-system/services/rancher-monitoring-prometheus:9090/proxy"

// New creates a Prometheus HTTP API client.
//
// If serverURL is empty, it defaults to the Prometheus instance managed by
// rancher-monitoring, accessed through the Kubernetes API-server proxy using
// the credentials from restConfig (bearer token, client certs, etc.).
//
// To connect directly (e.g. via kubectl port-forward), pass the URL explicitly:
//
//	client, err := New("http://localhost:9090", restConfig)
func New(serverURL string, restConfig *rest.Config) (promv1.API, error) {
	transport, err := rest.TransportFor(restConfig)
	if err != nil {
		return nil, err
	}
	client, err := promapi.NewClient(promapi.Config{
		Address:      resolvePrometheusURL(serverURL, restConfig),
		RoundTripper: transport,
	})
	if err != nil {
		return nil, err
	}
	return promv1.NewAPI(client), nil
}

func resolvePrometheusURL(serverURL string, restConfig *rest.Config) string {
	if serverURL == "" {
		return restConfig.Host + prometheusSVCProxy
	}

	return serverURL
}
