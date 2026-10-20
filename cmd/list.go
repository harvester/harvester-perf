package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/harvester/hvperf/pkg/suites"
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
	suites := suites.All()
	switch format {
	case "json":
		out, err := json.Marshal(suites)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(os.Stdout, "%s\n", strings.TrimSpace(string(out)))
		return err

	case "yaml":
		out, err := yaml.Marshal(suites)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(os.Stdout, "%s\n", strings.TrimSpace(string(out)))
		return err

	case "name":
		var raw []string
		for _, s := range suites {
			raw = append(raw, s.Name())
		}
		out := []byte(strings.Join(raw, "\n"))
		_, err := fmt.Fprintf(os.Stdout, "%s\n", strings.TrimSpace(string(out)))
		return err

	case "table":
		fallthrough
	default:
		rows := []string{"NAME\tREAD/WRITE\tDESCRIPTION"}
		for _, s := range suites {
			rows = append(rows, fmt.Sprintf("%s", s))
		}

		out := []byte(strings.Join(rows, "\n"))
		tab := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', 0)
		if _, err := fmt.Fprintf(tab, "%s\n", strings.TrimSpace(string(out))); err != nil {
			return err
		}
		return tab.Flush()
	}
}
