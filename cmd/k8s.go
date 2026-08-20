package cmd

import (
	kcliopts "k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
)

var (
	k8sConfigFlags = kcliopts.NewConfigFlags(true)
	k8sPrintFlags  = kcliopts.NewPrintFlags("perf")
)

func restConfig() (*rest.Config, error) {
	return k8sConfigFlags.ToRESTConfig()
}
