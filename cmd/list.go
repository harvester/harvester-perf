package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/harvester/hperf/internal/suites"
	"go.yaml.in/yaml/v4"

	"github.com/spf13/cobra"
	kcliopts "k8s.io/cli-runtime/pkg/genericclioptions"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List the performance test suites",
	Long: `List the performance test suites.

All "read-only" and "read-write" test suites are listed. Use the -o/--output flag
to specify the output format. Supported formats are "table", "name", "json", and
"yaml". The default format is "table".`,
	RunE: func(cmd *cobra.Command, args []string) error {
		format := cmd.Flag("output").Value.String()
		return listOut(format)
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
	kcliopts.NewPrintFlags("perf").AddFlags(listCmd)
	listCmd.SilenceUsage = true
}

func listOut(format string) error {
	suites := suites.All(true)

	var (
		data []byte
		err  error
	)
	switch format {
	case "json":
		data, err = json.Marshal(suites)
	case "yaml":
		data, err = yaml.Marshal(suites)
	case "name":
		var raw []string
		for _, s := range suites {
			raw = append(raw, s.Name())
		}
		data = []byte(strings.Join(raw, "\n"))
	case "table":
		fallthrough
	default:
		var raw []string
		for _, s := range suites {
			raw = append(raw, fmt.Sprintf("%s", s))
		}
		data = []byte(strings.Join(raw, "\n"))
	}
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(os.Stdout, "%s\n", strings.TrimSpace(string(data)))
	return err
}
