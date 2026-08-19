package cmd

import (
	"os"

	"github.com/spf13/cobra"
	kcliopts "k8s.io/cli-runtime/pkg/genericclioptions"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "hperf",
	Short: "hperf is a CLI tool for assessing Harvester performance and capacity benchmarks",
	Long: `hperf is a CLI tool for assessing Harvester performance and benchmarks.

It provides various commands to run test suites, collect metrics, and
analyze results.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	kcliopts.NewConfigFlags(true).AddFlags(runCmd.Flags())
}
