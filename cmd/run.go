/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

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
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("run called")
	},
}

var runAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Run all the performance test suites",
	Long: `Run all the performance test suites.

By default, only "read-only" test suites are run. These tests do not create or
modify any resources in the cluster. To include "read-write" test suites, use the
'--include-write' flag.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("run all called")
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.AddCommand(runAllCmd)
	runAllCmd.Flags().Bool("include-write", false, "Include read-write test suites")
	kcliopts.NewPrintFlags("perf").AddFlags(runCmd)
	for _, c := range runCmd.Commands() {
		kcliopts.NewPrintFlags("perf").AddFlags(c)
	}
}
