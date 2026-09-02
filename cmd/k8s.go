package cmd

import (
	pkgprom "github.com/harvester/hvperf/pkg/prometheus"
	"github.com/harvester/hvperf/pkg/suites"
	monclient "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
	kcliopts "k8s.io/cli-runtime/pkg/genericclioptions"
	discoveryclient "k8s.io/client-go/discovery"
	dynclient "k8s.io/client-go/dynamic"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	k8sConfigFlags = kcliopts.NewConfigFlags(true)
	k8sPrintFlags  = kcliopts.NewPrintFlags("perf")
)

func restConfig() (*rest.Config, error) {
	return k8sConfigFlags.ToRESTConfig()
}

func k8sClientSet() (k8sclient.Interface, error) {
	restConfig, err := restConfig()
	if err != nil {
		return nil, err
	}
	return k8sclient.NewForConfig(restConfig)
}

func dynClientSet() (dynclient.Interface, error) {
	restConfig, err := restConfig()
	if err != nil {
		return nil, err
	}
	return dynclient.NewForConfig(restConfig)
}

func monClientSet() (monclient.Interface, error) {
	restConfig, err := restConfig()
	if err != nil {
		return nil, err
	}
	return monclient.NewForConfig(restConfig)
}

func discoveryClient() (discoveryclient.DiscoveryInterface, error) {
	restConfig, err := restConfig()
	if err != nil {
		return nil, err
	}
	return discoveryclient.NewDiscoveryClientForConfig(restConfig)
}

// configureClients configures all the K8s client sets for the test
// suites. the client sets are configured using the K8s config flags that are
// passed to the command line. Hence, this function should be called after the
// command line flags are parsed.
func configureClients() (*suites.Clients, error) {
	restConfig, err := restConfig()
	if err != nil {
		return nil, err
	}

	k8sClientSet, err := k8sClientSet()
	if err != nil {
		return nil, err
	}

	dynClientSet, err := dynClientSet()
	if err != nil {
		return nil, err
	}

	monClientSet, err := monClientSet()
	if err != nil {
		return nil, err
	}

	promClient, err := pkgprom.NewPrometheusClient(restConfig)
	if err != nil {
		return nil, err
	}

	clients := suites.NewClients(k8sClientSet, dynClientSet, monClientSet, restConfig, promClient)
	return clients, nil
}
