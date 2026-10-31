package network

import (
	"context"
	"fmt"
	"time"

	"github.com/harvester/hvperf/internal/suites/options"
	"github.com/harvester/hvperf/pkg/k8s"
	"github.com/harvester/hvperf/pkg/suites"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
)

func init() {
	suites.Register(NewNetworkSuite())
}

var _ suites.Suite = &NetworkSuite{}

type NetworkSuite struct {
	suites.SuiteMarshaler
	*suites.ProgressReporter
	*suites.Clients
}

func NewNetworkSuite() *NetworkSuite {
	n := &NetworkSuite{}
	n.Marshal = n
	return n
}

func (s *NetworkSuite) Name() string {
	return "network-benchmark"
}

func (s *NetworkSuite) Description() string {
	return "run netperf to benchmark the in-cluster network latency and throughput"
}

func (s *NetworkSuite) IsExclusive() bool {
	return true
}

func (s *NetworkSuite) SetClients(clients *suites.Clients) {
	s.Clients = clients
}

func (s *NetworkSuite) SetProgressReporter(reporter *suites.ProgressReporter) {
	s.ProgressReporter = reporter
}

func (s *NetworkSuite) RunE(
	ctx context.Context,
	runID string,
	namespace string,
	opts suites.Options,
) suites.SuiteResult {
	o, err := NewNetPerfOptions(opts)
	if err != nil {
		return suites.NewSuiteResult(s.Name(), runID).WithError(err)
	}

	if _, err := k8s.EnsureNamespace(ctx, s.Clients, namespace, o.NamespaceReadyTimeout); err != nil {
		return suites.SuiteResult{
			Name:  s.Name(),
			RunID: runID,
			Err:   fmt.Sprintf("namespace %s not ready: %v", namespace, err.Error()),
		}
	}
	klog.V(3).InfoS("namespace is ready\n", "name", namespace)

	testSuites := []struct {
		name string
		fn   func(context.Context, string, string, string, *NetworkOptions) *suites.CaseResult
	}{
		{
			name: "network-benchmark-netperf",
			fn:   s.netperf,
		},
		{
			name: "network-benchmark-iperf3",
			fn:   s.iperf,
		},
	}

	var caseResults []*suites.CaseResult
	for _, testCase := range testSuites {
		s.CaseStart(s.Name(), testCase.name)
		caseResult := testCase.fn(ctx, testCase.name, runID, namespace, o)
		caseResults = append(caseResults, caseResult)
		s.CaseDone(s.Name(), testCase.name, caseResult.State, time.Since(caseResult.DateTimeStart))
	}

	suiteParams, err := suites.ToSuiteParams(o)
	if err != nil {
		// just log the params conversion error, don't fail the suite. the only effect
		// is that the suite params won't be reported in the results.
		klog.V(3).ErrorS(err, "failed to convert suite options to suite params\n")
	}

	return suites.NewSuiteResult(s.Name(), runID).WithParams(suiteParams).WithResults(caseResults)
}

func (s *NetworkSuite) netperf(
	ctx context.Context,
	caseName string,
	runID string,
	namespace string,
	o *NetworkOptions,
) *suites.CaseResult {
	start := time.Now()
	var (
		cmdResults    []*suites.CmdResult
		metricResults []*suites.MetricResult
		objs          []runtime.Object
	)
	end := time.Now()
	return suites.NewCaseResult(caseName, start, end, cmdResults, metricResults, objs...)
}

func (s *NetworkSuite) iperf(
	ctx context.Context,
	caseName string,
	runID string,
	namespace string,
	o *NetworkOptions,
) *suites.CaseResult {
	start := time.Now()
	var (
		cmdResults    []*suites.CmdResult
		metricResults []*suites.MetricResult
		objs          []runtime.Object
	)
	end := time.Now()
	return suites.NewCaseResult(caseName, start, end, cmdResults, metricResults, objs...)
}

type NetworkOptions struct {
	NamespaceReadyTimeout time.Duration
}

func NewNetPerfOptions(override suites.Options) (*NetworkOptions, error) {
	merged := options.DefaultGlobalOptions().
		Merge(options.DefaultNetworkOptions()).
		Merge(override)
	o, err := options.FromOptions[*NetworkOptions](merged)
	if err != nil {
		return nil, err
	}
	return o, nil
}
