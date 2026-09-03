package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/hvperf/pkg/suites"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"
)

var (
	runCmdClients *suites.Clients
	keepAlive     bool
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the performance test suites",
	Long: `Run the performance test suites.

Works with a single test suite or multiple test suites. The test suites can be
specified as a list of comma-separated arguments.

Use 'all' to run every registered test suite, both "read-only" and "read-write".
"read-write" test suites create or modify resources in the cluster.`,
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
		argSuites := strings.Split(args[0], ",")
		suites := suites.Find(argSuites)
		outputFormat := *k8sPrintFlags.OutputFormat
		results, err := runSuites(suites, outputFormat)
		return outRun(results, outputFormat, err)
	},
}

var runAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Run all the performance test suites",
	Long: `Run all the performance test suites.

Every registered test suite is run, both "read-only" and "read-write".
"read-write" test suites create or modify resources in the cluster.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		testSuites := suites.All()
		outputFormat := *k8sPrintFlags.OutputFormat
		results, err := runSuites(testSuites, outputFormat)
		return outRun(results, outputFormat, err)
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.AddCommand(runAllCmd)

	runCmd.SilenceUsage = true
	runAllCmd.SilenceUsage = true

	runCmd.PersistentFlags().BoolVar(&keepAlive, "keep-alive", true,
		fmt.Sprintf("Keep the test namespace and all its resources after test suite execution. Only works if the namespace is %s", suites.DefaultNamespace))

	k8sConfigFlags.AddFlags(runCmd.PersistentFlags())
	k8sPrintFlags.AddFlags(runCmd)
	for _, c := range runCmd.Commands() {
		k8sPrintFlags.AddFlags(c)
	}
}

func runSuites(testSuites []suites.Suite, format string) ([]*suites.SuiteResult, error) {
	var (
		errs    error
		results []*suites.SuiteResult

		ctx   = context.Background()
		runID = time.Now().Format("20060102150405")
	)

	namespace := suites.DefaultNamespace
	if k8sConfigFlags.Namespace != nil && *k8sConfigFlags.Namespace != "" {
		namespace = *k8sConfigFlags.Namespace
	}
	defer func() {
		if !keepAlive && namespace == suites.DefaultNamespace {
			//nolint:errcheck
			runCmdClients.K8sClientSet.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
		}
	}()

	for _, suite := range testSuites {
		suite = suites.WithClients(suite, runCmdClients)
		result, err := runSuite(ctx, runID, namespace, suite, suites.Options{})
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to run test suite %q: %w", suite.Name(), err))
			continue
		}
		results = append(results, &result)
	}
	return results, errs
}

func runSuite(ctx context.Context, runID, namespace string, testSuite suites.Suite, opts suites.Options) (suites.SuiteResult, error) {
	return testSuite.RunE(ctx, runID, namespace, opts)
}

// outRun outputs the results of the test suites in the specified format (json,
// yaml, or text). The slice of results is always marshaled and printed, even if
// there are errors. This ensures useful partial results are not discarded.
func outRun(results []*suites.SuiteResult, format string, runErr error) error {
	var (
		out []byte
		err error
	)
	switch format {
	case "json":
		out, err = json.Marshal(results)
	case "yaml":
		out, err = yaml.Marshal(results)
	case "text":
		fallthrough
	default:
		var s []string
		for _, result := range results {
			s = append(s, result.String())
		}
		out = []byte(strings.Join(s, "\n\n"))
	}
	if runErr != nil {
		err = errors.Join(runErr, err)
	}
	if _, formatErr := fmt.Fprintf(os.Stdout, "%s", out); formatErr != nil {
		err = errors.Join(formatErr, err)
	}
	return err
}
