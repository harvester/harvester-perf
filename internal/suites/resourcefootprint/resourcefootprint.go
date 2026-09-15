package resourcefootprint

import (
	"context"
	"fmt"
	"time"

	"github.com/harvester/hvperf/internal/suites/options"
	"github.com/harvester/hvperf/pkg/k8s"
	pkgprom "github.com/harvester/hvperf/pkg/prometheus"
	pkgsuites "github.com/harvester/hvperf/pkg/suites"
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

var _ pkgsuites.Suite = &ResourceFootprintSuite{}

func init() {
	suite := &ResourceFootprintSuite{}
	suite.Marshal = suite
	pkgsuites.Register(suite)
}

type ResourceFootprintSuite struct {
	pkgsuites.SuiteMarshaler
	*pkgsuites.Clients
}

type resourceFootprintOptions struct {
	MonitoringAddonName string
	MonitoringNamespace string
}

func (s *ResourceFootprintSuite) SetProgressReporter(_ *pkgsuites.ProgressReporter) {}
func (s *ResourceFootprintSuite) Name() string {
	return "resource-footprint"
}

func (s *ResourceFootprintSuite) Description() string {
	return "measure the resource footprint of the cluster"
}
func (s *ResourceFootprintSuite) IsReadWrite() bool                     { return false }
func (s *ResourceFootprintSuite) SetClients(clients *pkgsuites.Clients) { s.Clients = clients }

func (s *ResourceFootprintSuite) measure(ctx context.Context, queries ...string) *pkgsuites.CaseResult {
	caseResult := &pkgsuites.CaseResult{
		DateTimeStart: time.Now(),
	}
	metrics := make([]*pkgsuites.MetricResult, 0, len(queries))
	var failed bool
	for _, query := range queries {
		samples, warnings, err := pkgprom.RunInstant(ctx, s.Clients.PromClient, query)
		if err != nil {
			failed = true
		}
		metrics = append(metrics, &pkgsuites.MetricResult{
			Err:      err,
			Query:    query,
			Samples:  samples,
			Warnings: warnings,
		})
	}

	caseResult.DateTimeEnd = time.Now()
	caseResult.MetricResults = metrics
	caseResult.State = pkgsuites.CaseResultStatePass
	if failed {
		caseResult.State = pkgsuites.CaseResultStateFail
	}
	return caseResult
}

func (s *ResourceFootprintSuite) measureNamespaceUsage(ctx context.Context) *pkgsuites.CaseResult {
	return s.measure(ctx, queryNsCPU, queryNsMem)
}

func (s *ResourceFootprintSuite) measureNamespaceResourceRequests(ctx context.Context) *pkgsuites.CaseResult {
	return s.measure(ctx, queryRequestCPU, queryRequestMem)
}

func (s *ResourceFootprintSuite) measureHostMemory(ctx context.Context) *pkgsuites.CaseResult {
	return s.measure(ctx, queryHostMemTotal, queryHostMemAvailable)
}

func (s *ResourceFootprintSuite) measureNodeAllocatable(ctx context.Context) *pkgsuites.CaseResult {
	return s.measure(ctx, queryAllocCPU, queryAllocMem)
}

func (s *ResourceFootprintSuite) RunE(ctx context.Context, runID, _ string, _ pkgsuites.Options) (pkgsuites.SuiteResult, error) {
	cases := []struct {
		name    string
		measure func() *pkgsuites.CaseResult
	}{
		{
			name:    "per-namespace resource usage",
			measure: func() *pkgsuites.CaseResult { return s.measureNamespaceUsage(ctx) },
		},
		{
			name:    "per-namespace resource requests",
			measure: func() *pkgsuites.CaseResult { return s.measureNamespaceResourceRequests(ctx) },
		},
		{
			name:    "host memory",
			measure: func() *pkgsuites.CaseResult { return s.measureHostMemory(ctx) },
		},
		{
			name:    "node allocatable resources",
			measure: func() *pkgsuites.CaseResult { return s.measureNodeAllocatable(ctx) },
		},
	}

	monitoringOpts, err := options.FromOptions[*resourceFootprintOptions](pkgsuites.DefaultGlobalOptions())
	if err != nil {
		return pkgsuites.SuiteResult{
			Name:  s.Name(),
			RunID: runID,
			Err:   err.Error(),
		}, err
	}
	enabled, err := k8s.MonitoringEnabled(ctx, s.Clients, monitoringOpts.MonitoringNamespace, monitoringOpts.MonitoringAddonName)
	if err != nil {
		return pkgsuites.SuiteResult{
			Name:  s.Name(),
			RunID: runID,
			Err:   fmt.Sprintf("monitoring addon not enabled: %s", err.Error()),
		}, err
	}
	if !enabled {
		now := time.Now()
		results := make([]*pkgsuites.CaseResult, len(cases))
		for i, c := range cases {
			results[i] = &pkgsuites.CaseResult{
				CaseName:      c.name,
				DateTimeStart: now,
				DateTimeEnd:   now,
				State:         pkgsuites.CaseResultStateSkipped,
			}
		}
		return pkgsuites.SuiteResult{
			Name:    s.Name(),
			Err:     "monitoring addon not enabled",
			RunID:   runID,
			Results: results,
		}, nil
	}

	results := make([]*pkgsuites.CaseResult, 0, len(cases))
	for _, c := range cases {
		caseResult := c.measure()
		results = append(results, caseResult)
	}

	return pkgsuites.SuiteResult{
		Name:    s.Name(),
		RunID:   runID,
		Results: results,
	}, nil
}
