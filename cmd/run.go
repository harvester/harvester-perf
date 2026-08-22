package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/harvester/hperf/pkg/suites"

	"github.com/spf13/cobra"
)

var (
	runCmdClients *suites.Clients
	includeWrite  bool
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
	Args: cobra.MinimumNArgs(1),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		clientSets, err := configureClients()
		if err != nil {
			return err
		}
		runCmdClients = clientSets
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		suites := suites.Find(args)
		results, err := runSuites(suites)
		if err != nil {
			return err
		}
		return outRun(results)
	},
}

var runAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Run all the performance test suites",
	Long: `Run all the performance test suites.

By default, only "read-only" test suites are run. These tests do not create or
modify any resources in the cluster. To include "read-write" test suites, use the
'--include-write' flag.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		testSuites := suites.All(includeWrite)
		results, err := runSuites(testSuites)
		if err != nil {
			return err
		}
		return outRun(results)
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.AddCommand(runAllCmd)

	runCmd.SilenceUsage = true
	runAllCmd.SilenceUsage = true

	runAllCmd.Flags().BoolVar(&includeWrite, "include-write", false, "Include read-write test suites")
	k8sConfigFlags.AddFlags(runCmd.PersistentFlags())
	k8sPrintFlags.AddFlags(runCmd)
	for _, c := range runCmd.Commands() {
		k8sPrintFlags.AddFlags(c)
	}
}

func runSuites(testSuites []suites.Suite) ([]*suites.SuiteResult, error) {
	var (
		results []*suites.SuiteResult
		errs    error
	)
	ctx := context.Background()
	for _, suite := range testSuites {
		suite = suites.WithClients(suite, runCmdClients)
		result, err := runSuite(ctx, suite, suites.Options{})
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to run test suite %q: %w", suite.Name(), err))
			continue
		}
		results = append(results, &result)
	}
	return results, errs
}

func runSuite(ctx context.Context, testSuite suites.Suite, opts suites.Options) (suites.SuiteResult, error) {
	return testSuite.RunE(ctx, opts)
}

func outRun(results []*suites.SuiteResult) error {
	out, err := json.Marshal(results)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(os.Stdout, "%s\n", out)
	return err
}
