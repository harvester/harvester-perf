package options

import (
	"encoding/json"

	"github.com/harvester/hvperf/pkg/suites"
)

// FromOptions converts the given suites.Options to the specific options type V.
func FromOptions[V any](opts *suites.Options) (V, error) {
	var o V
	b, err := json.Marshal(opts)
	if err != nil {
		return o, err
	}

	if err := json.Unmarshal(b, &o); err != nil {
		return o, err
	}
	return o, nil
}
