package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"strings"
	"time"

	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/harvester/hvperf/internal/suites/options"
	"github.com/harvester/hvperf/pkg/suites"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"
)

var (
	runCmdClients        *suites.Clients
	keepAlive            bool
	monitoringServiceURL string
	configFile           string
	progress             *suites.ProgressReporter
	runOptions           = map[string]suites.Options{}
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the performance test suites",
	Long: `Run the performance test suites.

Works with a single test suite or multiple test suites. The test suites can be
specified as a list of comma-separated arguments.`,
	Args: cobra.MinimumNArgs(1),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		clientSets, err := configureClients()
		if err != nil {
			return err
		}
		runCmdClients = clientSets

		ro, err := loadRunOptions(configFile)
		if err != nil {
			return err
		}
		runOptions = ro

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		argSuites := strings.Split(args[0], ",")
		testSuites, err := suites.Find(argSuites)
		if err != nil {
			return err
		}

		toolsDir, err := toolsDirPath()
		if err != nil {
			return err
		}

		suitesOptions, err := canonicalizeRunOptions(testSuites)
		if err != nil {
			return err
		}

		outputFormat := *k8sPrintFlags.OutputFormat
		results := runSuites(cmd.Context(), testSuites, toolsDir, outputFormat, suitesOptions)
		return outRun(results, outputFormat)
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.SilenceUsage = true

	runCmd.PersistentFlags().BoolVar(&keepAlive, "keep-alive", false,
		fmt.Sprintf("Keep the test namespace and all its resources after test suite execution. Only works if the namespace is %s. Cleanup is skipped for custom namespace", suites.DefaultNamespace))
	runCmd.PersistentFlags().StringVar(&monitoringServiceURL, "monitoring-url", "",
		"Prometheus HTTP API base URL. Defaults to the in-cluster Prometheus service proxy.")
	runCmd.PersistentFlags().StringVar(&configFile, "config", "",
		"Path to config file (YAML) used to override test suite default parameters. If omitted, the test suite default parameters are used.")

	k8sConfigFlags.AddFlags(runCmd.PersistentFlags())
	k8sPrintFlags.AddFlags(runCmd)
	for _, c := range runCmd.Commands() {
		k8sPrintFlags.AddFlags(c)
	}
}

func runSuites(ctx context.Context, testSuites []suites.Suite, toolsDir string, format string, suitesOptions map[string]suites.Options) []*suites.SuiteResult {
	var (
		results []*suites.SuiteResult
		runID   = time.Now().Format("20060102150405")
	)

	namespace := suites.DefaultNamespace
	if k8sConfigFlags.Namespace != nil && *k8sConfigFlags.Namespace != "" {
		namespace = *k8sConfigFlags.Namespace
	}
	defer func() {
		if !keepAlive && namespace == suites.DefaultNamespace {
			if err := runCmdClients.K8sClientSet.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{}); err != nil {
				if !apierrs.IsNotFound(err) {
					klog.ErrorS(err, "failed to delete test namespace", "namespace", namespace)
				}
			}
			return
		}
		klog.InfoS("skipping test namespace cleanup", "namespace", namespace, "keepAlive", keepAlive)
	}()
	progress = suites.NewProgressReporter(os.Stderr, runID, len(testSuites))

	for i, suite := range testSuites {
		suiteOptions := suites.Options{}
		if o, exists := suitesOptions["global"]; exists {
			maps.Copy(suiteOptions, o)
		}
		if o, exists := suitesOptions[suite.Name()]; exists {
			maps.Copy(suiteOptions, o)
		}

		// every test suite gets toolsDir, needed by suites identify the location of
		// tools like etcdctl, netperf etc. it is not overridable
		suiteOptions["toolsDir"] = toolsDir

		suite = suites.WithClients(suite, runCmdClients)
		suite = suites.WithProgressReporter(suite, progress)
		result := runSuite(ctx, runID, namespace, suite, i+1, suiteOptions)
		results = append(results, &result)
	}
	return results
}

func runSuite(ctx context.Context, runID, namespace string, testSuite suites.Suite, i int, opts suites.Options) suites.SuiteResult {
	start := time.Now()
	progress.SuiteStart(testSuite.Name(), i)
	defer func() {
		progress.SuiteDone(testSuite.Name(), i, time.Since(start))
	}()

	return testSuite.RunE(ctx, runID, namespace, opts)
}

// outRun outputs the results of the test suites in the specified format (json,
// yaml, or text). The slice of results is always marshaled and printed, even if
// there are errors. This ensures useful partial results are not discarded.
func outRun(results []*suites.SuiteResult, format string) error {
	var (
		out []byte
		err error
	)
	switch format {
	case "json":
		out, err = json.Marshal(results)
		if err != nil {
			return err
		}
	case "yaml":
		out, err = yaml.Marshal(results)
		if err != nil {
			return err
		}
	case "text":
		fallthrough
	default:
		var s []string
		for _, result := range results {
			s = append(s, result.String())
		}
		out = []byte(strings.Join(s, "\n"))
	}
	_, err = fmt.Fprintf(os.Stdout, "%s", out)
	return err
}

// canonicalizeRunOptions takes the user-provided run options and canonicalizes
// (standardizes) them against the default options for each test suite. These
// default options serves as the schema.
//
// Global options are also canonicalized against the default global options
// schema, if they exist under the 'global' key in the user-provided run options.
//
// The returned map contains a map of the canonicalized options with the same set
// of keys as the user-provided run options, but with the values canonicalized
// against the schema.
func canonicalizeRunOptions(testSuites []suites.Suite) (map[string]suites.Options, error) {
	if runOptions == nil {
		return map[string]suites.Options{}, nil
	}

	canonicalizedOptions := map[string]suites.Options{}

	// canonicalize any user-provided global options if they exist
	if globalOptions, exists := runOptions["global"]; exists {
		canonicalized, err := globalOptions.Canonicalize(options.DefaultGlobalOptions())
		if err != nil {
			return nil, fmt.Errorf("failed to canonicalize global options: %w", err)
		}
		canonicalizedOptions["global"] = canonicalized
	}

	// for each suite, canonicalize the user-provided options if they exist, but
	// first identify and error on unknown suites in the override options
	unknownSuites := []string{}
	for suiteName := range runOptions {
		known := false
		for _, testSuite := range testSuites {
			known = suiteName == testSuite.Name() || suiteName == "global"
			if known {
				break
			}
		}
		if !known {
			unknownSuites = append(unknownSuites, suiteName)
		}
	}
	if len(unknownSuites) > 0 {
		return nil, fmt.Errorf("unsupported suites in override options: %s", strings.Join(unknownSuites, ","))
	}

	for _, suite := range testSuites {
		canonicalized, err := runOptions[suite.Name()].Canonicalize(suite.OptionsSchema())
		if err != nil {
			return nil, fmt.Errorf("failed to canonicalize options for suite %q: %w", suite.Name(), err)
		}
		canonicalizedOptions[suite.Name()] = canonicalized
	}

	return canonicalizedOptions, nil
}
