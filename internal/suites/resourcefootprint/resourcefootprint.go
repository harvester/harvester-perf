package resourcefootprint

import (
	"context"
	"fmt"
	"time"

	"github.com/harvester/hvperf/internal/suites/options"
	"github.com/harvester/hvperf/pkg/k8s"
	pkgprom "github.com/harvester/hvperf/pkg/prometheus"
	"github.com/harvester/hvperf/pkg/suites"
)

const (
	footprintNamespaceFilter = `namespace=~"kube-system|kubevirt|cdi|longhorn-system|cattle-monitoring-system|cattle-logging-system|harvester-system|fleet-local"`

	// Per-namespace usage.
	queryNsCPU = `sum by (namespace, node) (rate(container_cpu_usage_seconds_total{container!="", pod!="", ` + footprintNamespaceFilter + `}[5m]))`
	queryNsMem = `avg_over_time((sum by (namespace, node) (container_memory_working_set_bytes{container!="", pod!="", ` + footprintNamespaceFilter + `}))[5m:30s])`

	// Per-namespace requests.
	queryRequestCPU = `sum by (namespace, node) (kube_pod_container_resource_requests{resource="cpu", node!="", ` + footprintNamespaceFilter + `})`
	queryRequestMem = `sum by (namespace, node) (kube_pod_container_resource_requests{resource="memory", node!="", ` + footprintNamespaceFilter + `})`

	// Host memory — kernel view.
	queryHostMemTotal     = `node_memory_MemTotal_bytes * on(instance) group_left(nodename) node_uname_info`
	queryHostMemAvailable = `node_memory_MemAvailable_bytes * on(instance) group_left(nodename) node_uname_info`

	// Allocatable.
	queryAllocCPU = `kube_node_status_allocatable{resource="cpu"}`
	queryAllocMem = `kube_node_status_allocatable{resource="memory"}`
)

var _ suites.Suite = &ResourceFootprintSuite{}

func init() {
	suite := &ResourceFootprintSuite{}
	suite.Marshal = suite
	suites.Register(suite)
}

type ResourceFootprintSuite struct {
	suites.SuiteMarshaler
	*suites.Clients
	*suites.ProgressReporter
}

type resourceFootprintOptions struct {
	MonitoringAddonName string
	MonitoringNamespace string
}

func (s *ResourceFootprintSuite) SetProgressReporter(progressReporter *suites.ProgressReporter) {
	s.ProgressReporter = progressReporter
}

func (s *ResourceFootprintSuite) Name() string {
	return "control-plane-footprint"
}

func (s *ResourceFootprintSuite) Description() string {
	return "measure the resource footprint of the cluster"
}
func (s *ResourceFootprintSuite) IsExclusive() bool                  { return false }
func (s *ResourceFootprintSuite) SetClients(clients *suites.Clients) { s.Clients = clients }

func (s *ResourceFootprintSuite) measure(ctx context.Context, queries ...string) *suites.CaseResult {
	metrics := make([]*suites.MetricResult, 0, len(queries))
	start := time.Now()
	for _, query := range queries {
		samples, warnings, err := pkgprom.RunInstant(ctx, s.Clients.PromClient, query)
		metric := &suites.MetricResult{
			Query:    query,
			Samples:  samples,
			Warnings: warnings,
		}
		if err != nil {
			metric.Err = err.Error()
		}
		metrics = append(metrics, metric)
	}

	return suites.NewCaseResult("", start, time.Now(), nil, metrics)
}

func (s *ResourceFootprintSuite) measureNamespaceUsage(ctx context.Context) *suites.CaseResult {
	return s.measure(ctx, queryNsCPU, queryNsMem)
}

func (s *ResourceFootprintSuite) measureNamespaceResourceRequests(ctx context.Context) *suites.CaseResult {
	return s.measure(ctx, queryRequestCPU, queryRequestMem)
}

func (s *ResourceFootprintSuite) measureHostMemory(ctx context.Context) *suites.CaseResult {
	return s.measure(ctx, queryHostMemTotal, queryHostMemAvailable)
}

func (s *ResourceFootprintSuite) measureNodeAllocatable(ctx context.Context) *suites.CaseResult {
	return s.measure(ctx, queryAllocCPU, queryAllocMem)
}

func (s *ResourceFootprintSuite) RunE(ctx context.Context, runID, _ string, opts suites.Options) suites.SuiteResult {
	cases := []struct {
		name    string
		measure func() *suites.CaseResult
	}{
		{
			name:    "per-namespace resource usage",
			measure: func() *suites.CaseResult { return s.measureNamespaceUsage(ctx) },
		},
		{
			name:    "per-namespace resource requests",
			measure: func() *suites.CaseResult { return s.measureNamespaceResourceRequests(ctx) },
		},
		{
			name:    "host memory",
			measure: func() *suites.CaseResult { return s.measureHostMemory(ctx) },
		},
		{
			name:    "node allocatable resources",
			measure: func() *suites.CaseResult { return s.measureNodeAllocatable(ctx) },
		},
	}

	merged := options.DefaultGlobalOptions().Merge(opts)
	monitoringOpts, err := options.FromOptions[*resourceFootprintOptions](merged)
	if err != nil {
		return suites.NewSuiteResult(s.Name(), runID).WithError(fmt.Errorf("failed to parse options: %w", err))
	}
	enabled, err := k8s.MonitoringEnabled(ctx, s.Clients, monitoringOpts.MonitoringNamespace, monitoringOpts.MonitoringAddonName)
	if err != nil {
		return suites.NewSuiteResult(s.Name(), runID).WithError(fmt.Errorf("failed to check if monitoring is enabled: %w", err))
	}
	if !enabled {
		return suites.NewSuiteResult(s.Name(), runID).WithError(fmt.Errorf("monitoring add-on %s not enabled in namespace %s", monitoringOpts.MonitoringAddonName, monitoringOpts.MonitoringNamespace))
	}

	results := make([]*suites.CaseResult, 0, len(cases))
	for _, c := range cases {
		s.CaseStart(s.Name(), c.name)
		caseResult := c.measure()
		caseResult.CaseName = c.name
		results = append(results, caseResult)
		s.CaseDone(s.Name(), c.name, caseResult.State, time.Since(caseResult.DateTimeStart))
	}

	return suites.NewSuiteResult(s.Name(), runID).WithResults(results)
}
