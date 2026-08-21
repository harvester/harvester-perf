package cmd

import (
	"github.com/harvester/hperf/pkg/suites"
	kcliopts "k8s.io/cli-runtime/pkg/genericclioptions"
	discoveryclient "k8s.io/client-go/discovery"
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
	k8sClientSet, err := k8sClientSet()
	if err != nil {
		return nil, err
	}
	return suites.NewClients(k8sClientSet), nil
}
