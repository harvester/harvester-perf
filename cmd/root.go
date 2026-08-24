package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "hvperf",
	Short: "hvperf is a CLI tool for assessing Harvester performance and capacity benchmarks",
	Long: `hvperf is a CLI tool for assessing Harvester performance and benchmarks.

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
