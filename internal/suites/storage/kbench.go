package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/harvester/hvperf/internal/suites/options"
	"github.com/harvester/hvperf/pkg/suites"
)

func init() {
	suites.Register(NewKbenchSuite())
}

var _ suites.Suite = &KBenchSuite{}

type KBenchSuite struct {
	suites.SuiteMarshaler
	*suites.ProgressReporter
	*suites.Clients
}

func NewKbenchSuite() *KBenchSuite {
	k := &KBenchSuite{}
	k.Marshal = k
	return k
}

func (k *KBenchSuite) Name() string {
	return "storage-kbench"
}

func (k *KBenchSuite) Description() string {
	return "run kbench to benchmark the storage performance of the cluster"
}

func (k *KBenchSuite) IsReadWrite() bool {
	return true
}

func (k *KBenchSuite) RunE(
	ctx context.Context,
	runID string,
	namespace string,
	opts suites.Options,
) suites.SuiteResult {
	o, err := KBenchOptionsDefaults()
	if err != nil {
		return suites.SuiteResult{
			Name:  k.Name(),
			RunID: runID,
			Err:   err.Error(),
		}
	}
	fmt.Printf("Running kbench suite with options: %+v\n", o)

	result := suites.SuiteResult{}

	return result
}

func (k *KBenchSuite) SetClients(clients *suites.Clients) {
	k.Clients = clients
}

func (k *KBenchSuite) SetProgressReporter(reporter *suites.ProgressReporter) {
	k.ProgressReporter = reporter
}

type KBenchOptions struct {
	CPUIdleProf        CPUIdleProfType
	BandwidthBlockSize string
	BandwidthIoDepth   int
	BandwidthNumJobs   int
	IOPSBlockSize      string
	IOPSIoDepth        int
	IOPSNumJobs        int
	LatencyBlockSize   string
	LatencyIoDepth     int
	LatencyNumJobs     int
	Mode               ModeType
	RampTime           time.Duration
	RunTime            time.Duration
	TestSize           string

	JobActiveDeadline      time.Duration
	JobPodContainerName    string
	JobPodImageName        string
	JobPodImageTag         string
	JobPodTTLAfterFinished time.Duration
	JobPodReadyTimeout     time.Duration
	JobSuspend             bool
}

// KBenchOptionsDefaults returns the default options for the kbench test suite.
// It merges the default global options with the default kbench options, giving
// precedence to the global options for job-related settings.
func KBenchOptionsDefaults() (*KBenchOptions, error) {
	sysOpts, err := options.FromOptions[*KBenchOptions](options.DefaultGlobalOptions())
	if err != nil {
		return nil, err
	}

	o, err := options.FromOptions[*KBenchOptions](options.DefaultKBenchOptions())
	if err != nil {
		return nil, err
	}
	o.JobActiveDeadline = sysOpts.JobActiveDeadline
	o.JobPodContainerName = sysOpts.JobPodContainerName
	o.JobPodTTLAfterFinished = sysOpts.JobPodTTLAfterFinished
	o.JobPodReadyTimeout = sysOpts.JobPodReadyTimeout
	o.JobSuspend = sysOpts.JobSuspend
	return o, nil
}

type CPUIdleProfType string

const (
	CPUIdleProfDisabled CPUIdleProfType = "disabled"
	CPUIdleProfEnabled  CPUIdleProfType = "enabled"
)

type ModeType string

const (
	ModeFull              ModeType = "full"
	ModeQuick             ModeType = "quick"
	ModeRandReadIOPS      ModeType = "random-read-iops"
	ModeRandReadLatency   ModeType = "random-read-latency"
	ModeRandWriteIOPS     ModeType = "random-write-iops"
	ModeRandWriteLatency  ModeType = "random-write-latency"
	ModeSeqReadBandwidth  ModeType = "sequential-read-bandwidth"
	ModeSeqWriteBandwidth ModeType = "sequential-write-bandwidth"
)
