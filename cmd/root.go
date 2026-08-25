package cmd

import (
	"context"
	"flag"
	"io"

	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "hvperf",
	Short: "hvperf is a CLI tool for assessing Harvester performance and capacity benchmarks",
	Long: `hvperf is a CLI tool for assessing Harvester performance and benchmarks.

It provides various commands to run test suites, collect metrics, and
analyze results.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if quiet {
			klog.SetOutput(io.Discard)
			klog.LogToStderr(false)
		}
	},
	Run: func(cmd *cobra.Command, args []string) {},
}

var quiet bool

func init() {
	// make sure root's pre-run and post-run hooks are executed for all subcommands
	cobra.EnableTraverseRunHooks = true

	state := klog.CaptureState()
	defer state.Restore()

	fs := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(fs)

	rootCmd.PersistentFlags().AddGoFlagSet(fs)
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "set to true to disable klog output")
}

// ExecuteContext adds all child commands to the root command and sets flags
// appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func ExecuteContext(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}
