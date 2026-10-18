package cmd

import (
	"fmt"
	"os"

	"github.com/harvester/hvperf/pkg/suites"
	"go.yaml.in/yaml/v4"
)

// loadRunOptions loads the run options from the specified YAML file and returns
// a map of suite names to their corresponding options.
func loadRunOptions(path string) (map[string]suites.Options, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}

	override := map[string]suites.Options{}
	if err := yaml.Unmarshal(data, &override); err != nil {
		return nil, fmt.Errorf("failed to unmarshal yaml: %w", err)
	}
	return override, nil
}
