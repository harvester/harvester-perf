package prometheus

import (
	"fmt"

	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"

	"k8s.io/client-go/rest"
)

func NewPrometheusClient(cfg *rest.Config) (promv1.API, error) {
	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, err
	}

	address := fmt.Sprintf(
		"%s/api/v1/namespaces/cattle-monitoring-system/services/rancher-monitoring-prometheus:9090/proxy",
		cfg.Host,
	)

	client, err := promapi.NewClient(promapi.Config{
		Address: address,
		Client:  httpClient,
	})
	if err != nil {
		return nil, err
	}

	return promv1.NewAPI(client), nil
}
