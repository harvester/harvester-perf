package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	discclient "k8s.io/client-go/discovery"
)

var (
	Version = "dev"

	clientOnly bool

	// versionCmd represents the version command
	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Display the version of hperf",
		Long:  `Display the version of hperf.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := version()
			if err != nil {
				return err
			}
			_, ferr := fmt.Fprint(os.Stdout, out)
			return ferr
		},
	}
)

func init() {
	rootCmd.AddCommand(versionCmd)
	versionCmd.SilenceUsage = true

	versionCmd.Flags().BoolVar(&clientOnly, "client-only", false, "Only display the client version, without querying the server.")
	k8sConfigFlags.AddFlags(versionCmd.Flags())
}

func version() (string, error) {
	if clientOnly {
		return fmt.Sprintf("Client version: %s\n", Version), nil
	}

	config, err := restConfig()
	if err != nil {
		return "", err
	}
	dc, err := discclient.NewDiscoveryClientForConfig(config)
	if err != nil {
		return "", err
	}
	serverVersion, err := dc.ServerVersion()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Client version: %s\nServer version: %s\n", Version, serverVersion), nil
}
