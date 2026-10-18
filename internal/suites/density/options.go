package density

import (
	"errors"
	"fmt"
	"time"

	"github.com/harvester/hvperf/internal/suites/options"
	"github.com/harvester/hvperf/pkg/resource"
	"github.com/harvester/hvperf/pkg/suites"
)

// Options is the density block of hvperf.yaml.
type Options struct {
	BatchSize             int           `yaml:"batchSize"`
	BatchWaitTimeout      time.Duration `yaml:"batchWaitTimeout"`
	EtcdNamespace         string        `yaml:"etcdNamespace"`
	MaxVMs                int           `yaml:"maxVMs"`
	NamespaceReadyTimeout time.Duration `yaml:"namespaceReadyTimeout"`
	VMI                   resource.VMI  `yaml:"vmi"`
}

func NewOptions(override suites.Options) (Options, error) {
	opts := options.DefaultGlobalOptions().
		Merge(options.DefaultVMDensityOptions()).
		Merge(override)
	return options.FromOptions[Options](opts)
}

func (o Options) Validate() error {
	if o.MaxVMs < 0 {
		return fmt.Errorf("maxVMs must be non-negative, got %d", o.MaxVMs)
	}
	if o.BatchSize <= 0 {
		return fmt.Errorf("batchSize must be positive, got %d", o.BatchSize)
	}
	if o.BatchWaitTimeout <= 0 {
		return fmt.Errorf("batchWaitTimeout must be positive, got %s", o.BatchWaitTimeout)
	}
	if o.VMI.ContainerDisk == "" {
		return errors.New("vmi.containerDisk must not be empty")
	}
	return nil
}
