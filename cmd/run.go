package cmd

import (
	"errors"
	"fmt"

	"github.com/harvester/hperf/internal/suites"

	"github.com/spf13/cobra"
	kcliopts "k8s.io/cli-runtime/pkg/genericclioptions"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the performance test suites",
	Long: `Run the performance test suites.

Works with a single test suite or multiple test suites. The test suites can be
specified as a list of comma-separated arguments.

Use 'all' to run all test suites. By default, only "read-only" test suites are
run. These tests do not create or modify any resources in the cluster. To include 
"read-write" test suites, use 'all --include-write' flag.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		suites := suites.Find(args)
		results, errs := runSuites(suites)
		return out(results, errs)
	},
}

var includeWrite bool

var runAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Run all the performance test suites",
	Long: `Run all the performance test suites.

By default, only "read-only" test suites are run. These tests do not create or
modify any resources in the cluster. To include "read-write" test suites, use the
'--include-write' flag.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		suites := suites.All(includeWrite)
		results, errs := runSuites(suites)
		return out(results, errs)
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.AddCommand(runAllCmd)
	runCmd.SilenceUsage = true
	runAllCmd.SilenceUsage = true
	runAllCmd.Flags().BoolVar(&includeWrite, "include-write", false, "Include read-write test suites")
	kcliopts.NewPrintFlags("perf").AddFlags(runCmd)
	for _, c := range runCmd.Commands() {
		kcliopts.NewPrintFlags("perf").AddFlags(c)
	}
}

type TestResult struct{}

func runSuites(suites []suites.TestSuite) ([]*TestResult, error) {
	var (
		results []*TestResult
		errs    error
	)
	for _, suite := range suites {
		result, err := runSuite(suite)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to run test suite %q: %w", suite, err))
			continue
		}
		results = append(results, result)
	}
	return results, errs
}

func runSuite(suite suites.TestSuite) (*TestResult, error) {
	return nil, nil
}

func out(results []*TestResult, errs error) error {
	return nil
}
