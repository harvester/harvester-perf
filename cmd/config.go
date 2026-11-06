package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/harvester/hvperf/internal/suites/options"
	"github.com/harvester/hvperf/pkg/suites"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"
	"k8s.io/klog/v2"
)

var (
	configFilePath string

	configCmd = &cobra.Command{
		Use:   "config",
		Short: "Support user-facing configuration for overriding test suites parameters",
		Long:  `Support user-facing configuration for overriding test suites parameters.`,
	}

	configTemplateCmd = &cobra.Command{
		Use:   "template",
		Args:  cobra.NoArgs,
		Short: "Generate a configuration template for overriding test suites parameters",
		Long: `Generate a configuration template for overriding test suites parameters.

The configuration file can be used with 'run --config <config-filepath>' to override the default parameters of the test suites.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return configTemplate()
		},
	}

	configValidateCmd = &cobra.Command{
		Use:   "validate",
		Args:  cobra.NoArgs,
		Short: "Validate a configuration file against the test suites schemas",
		Long: `Validate a configuration file against the test suites schemas.

Use this command to validate the configuration file before passing it to the 'run' command.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return configValidate()
		},
	}
)

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configTemplateCmd)
	configCmd.AddCommand(configValidateCmd)
	configValidateCmd.Flags().StringVarP(&configFilePath, "config", "c", "", "Path to the configuration file to validate")
	if err := configValidateCmd.MarkFlagRequired("config"); err != nil {
		klog.V(3).ErrorS(err, "failed to mark 'config' flag as required")
	}
}

func configTemplate() error {
	var b strings.Builder
	tmpl := map[string]suites.Options{}
	for _, suite := range suites.All() {
		schema := suite.OptionsSchema()
		if len(schema) == 0 {
			continue
		}
		tmpl[suite.Name()] = schema
	}
	tmpl["global"] = options.DefaultGlobalOptions()

	y, err := yaml.Dump(tmpl, yaml.WithIndent(2))
	if err != nil {
		return err
	}
	if _, err := b.WriteString(string(y)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(os.Stdout, "%s", b.String()); err != nil {
		return err
	}
	return nil
}

func configValidate() error {
	opts, err := loadRunOptions(configFilePath)
	if err != nil {
		return err
	}
	runOptions = opts
	if _, err := canonicalizeRunOptions(suites.All()); err != nil {
		fmt.Printf("✗ errors in configuration file %s\n", configFilePath)
		return err
	}
	fmt.Printf("✓ configuration file %s is valid\n", configFilePath)
	return nil
}
