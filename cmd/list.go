package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/harvester/hperf/pkg/suites"
	"go.yaml.in/yaml/v4"

	"github.com/spf13/cobra"
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
		return outList(*k8sPrintFlags.OutputFormat)
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
	k8sPrintFlags.AddFlags(listCmd)
	listCmd.SilenceUsage = true
}

func outList(format string) error {
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
