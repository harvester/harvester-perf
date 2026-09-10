package resourcefootprint

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	pkgprom "github.com/harvester/hvperf/pkg/prometheus"
	pkgsuites "github.com/harvester/hvperf/pkg/suites"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	// Per-namespace usage
	queryNsCPU = `sum by (namespace, node) (rate(container_cpu_usage_seconds_total{container!="", pod!=""}[5m]))`
	queryNsMem = `avg_over_time((sum by (namespace, node) (container_memory_working_set_bytes{container!="", pod!=""}))[5m:30s])`

	// Per-node requests
	queryRequestCPU = `sum by (node) (kube_pod_container_resource_requests{resource="cpu", node!=""})`
	queryRequestMem = `sum by (node) (kube_pod_container_resource_requests{resource="memory", node!=""})`

	// Host memory — kernel view
	queryHostMemTotal     = `node_memory_MemTotal_bytes * on(instance) group_left(nodename) node_uname_info`
	queryHostMemAvailable = `node_memory_MemAvailable_bytes * on(instance) group_left(nodename) node_uname_info`

	// Allocatable
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

func (s *ResourceFootprintSuite) SetProgressReporter(_ *pkgsuites.ProgressReporter) {}
func (s *ResourceFootprintSuite) Name() string {
	return "resource-footprint"
}

func (s *ResourceFootprintSuite) Description() string {
	return "measure the static resource footprint of the cluster"
}
func (s *ResourceFootprintSuite) IsReadWrite() bool                     { return false }
func (s *ResourceFootprintSuite) SetClients(clients *pkgsuites.Clients) { s.Clients = clients }

func (s *ResourceFootprintSuite) addWarnings(sb *strings.Builder, query string, warnings promv1.Warnings) {
	if len(warnings) == 0 {
		return
	}
	fmt.Fprintf(sb, "# warning [query=%s]:\n", query)
	for _, w := range warnings {
		fmt.Fprintf(sb, "#   - %s\n", w)
	}
}

// measureNamespaceUsage reports per-namespace CPU and memory usage.
func (s *ResourceFootprintSuite) measureNamespaceUsage(ctx context.Context) (string, []*pkgsuites.MetricResult, []string, error) {
	var sb strings.Builder
	queries := []string{queryNsCPU, queryNsMem}

	cpuVec, cpuWarn, err := pkgprom.RunInstant(ctx, s.Clients.PromClient, queryNsCPU)
	s.addWarnings(&sb, queryNsCPU, cpuWarn)
	if err != nil {
		return sb.String(), nil, queries, err
	}

	memVec, memWarn, err := pkgprom.RunInstant(ctx, s.Clients.PromClient, queryNsMem)
	s.addWarnings(&sb, queryNsMem, memWarn)
	if err != nil {
		return sb.String(), nil, queries, err
	}

	// index mem by namespace/node
	memByKey := make(map[string]*resource.Quantity, len(memVec))
	for _, sample := range memVec {
		ns := string(sample.Metric["namespace"])
		node := string(sample.Metric["node"])
		memByKey[ns+"/"+node] = pkgprom.FormatMiB(float64(sample.Value))
	}

	// sort by namespace then node
	sort.Slice(cpuVec, func(i, j int) bool {
		nsi := string(cpuVec[i].Metric["namespace"])
		nsj := string(cpuVec[j].Metric["namespace"])
		if nsi != nsj {
			return nsi < nsj
		}
		return string(cpuVec[i].Metric["node"]) < string(cpuVec[j].Metric["node"])
	})

	tw := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAMESPACE\tNODE\tCPU(usage)\tMEMORY(usage)\t")
	fmt.Fprintln(tw, strings.Repeat("-", 9)+"\t"+strings.Repeat("-", 4)+"\t"+strings.Repeat("-", 10)+"\t"+strings.Repeat("-", 13)+"\t")
	for _, sample := range cpuVec {
		ns := string(sample.Metric["namespace"])
		node := string(sample.Metric["node"])
		cpuQty := pkgprom.FormatMilliCPU(float64(sample.Value))
		memQty := memByKey[ns+"/"+node]
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t\n", ns, node, cpuQty.String(), memQty.String())
	}
	tw.Flush()

	metrics := []*pkgsuites.MetricResult{
		{Query: queryNsCPU, Samples: cpuVec},
		{Query: queryNsMem, Samples: memVec},
	}
	return sb.String(), metrics, queries, nil
}

// measureNodeResourceRequests reports requested CPU and memory per node.
func (s *ResourceFootprintSuite) measureNodeResourceRequests(ctx context.Context) (string, []*pkgsuites.MetricResult, []string, error) {
	var sb strings.Builder
	queries := []string{queryRequestCPU, queryRequestMem}

	cpuVec, cpuWarn, err := pkgprom.RunInstant(ctx, s.Clients.PromClient, queryRequestCPU)
	s.addWarnings(&sb, queryRequestCPU, cpuWarn)
	if err != nil {
		return sb.String(), nil, queries, err
	}
	memVec, memWarn, err := pkgprom.RunInstant(ctx, s.Clients.PromClient, queryRequestMem)
	s.addWarnings(&sb, queryRequestMem, memWarn)
	if err != nil {
		return sb.String(), nil, queries, err
	}

	memByNode := make(map[string]float64, len(memVec))
	for _, sample := range memVec {
		memByNode[string(sample.Metric["node"])] = float64(sample.Value)
	}
	sort.Slice(cpuVec, func(i, j int) bool {
		return string(cpuVec[i].Metric["node"]) < string(cpuVec[j].Metric["node"])
	})

	tw := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NODE\tCPU REQUEST\tMEMORY REQUEST\t")
	fmt.Fprintln(tw, strings.Repeat("-", 4)+"\t"+strings.Repeat("-", 11)+"\t"+strings.Repeat("-", 14)+"\t")
	for _, sample := range cpuVec {
		node := string(sample.Metric["node"])
		fmt.Fprintf(tw, "%s\t%s\t%s\t\n", node, pkgprom.FormatMilliCPU(float64(sample.Value)), pkgprom.FormatMiB(memByNode[node]))
	}
	tw.Flush()

	metrics := []*pkgsuites.MetricResult{
		{Query: queryRequestCPU, Samples: cpuVec},
		{Query: queryRequestMem, Samples: memVec},
	}
	return sb.String(), metrics, queries, nil
}

// measureHostMemory reports total, available, and used host memory per node-exporter instance.
func (s *ResourceFootprintSuite) measureHostMemory(ctx context.Context) (string, []*pkgsuites.MetricResult, []string, error) {
	var sb strings.Builder
	queries := []string{queryHostMemTotal, queryHostMemAvailable}

	totalVec, totalWarn, err := pkgprom.RunInstant(ctx, s.Clients.PromClient, queryHostMemTotal)
	s.addWarnings(&sb, queryHostMemTotal, totalWarn)
	if err != nil {
		return sb.String(), nil, queries, err
	}

	availableVec, availableWarn, err := pkgprom.RunInstant(ctx, s.Clients.PromClient, queryHostMemAvailable)
	s.addWarnings(&sb, queryHostMemAvailable, availableWarn)
	if err != nil {
		return sb.String(), nil, queries, err
	}

	availableByInstance := make(map[string]float64, len(availableVec))
	for _, sample := range availableVec {
		availableByInstance[string(sample.Metric["nodename"])] = float64(sample.Value)
	}

	sort.Slice(totalVec, func(i, j int) bool {
		return string(totalVec[i].Metric["nodename"]) < string(totalVec[j].Metric["nodename"])
	})

	tw := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "INSTANCE\tTOTAL\tAVAILABLE\tUSED\t")
	fmt.Fprintln(tw, strings.Repeat("-", 8)+"\t"+strings.Repeat("-", 5)+"\t"+strings.Repeat("-", 9)+"\t"+strings.Repeat("-", 4)+"\t")
	for _, sample := range totalVec {
		instance := string(sample.Metric["nodename"])
		total := float64(sample.Value)
		available := availableByInstance[instance]
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t\n", instance, pkgprom.FormatMiB(total), pkgprom.FormatMiB(available), pkgprom.FormatMiB(total-available))
	}
	tw.Flush()

	metrics := []*pkgsuites.MetricResult{
		{Query: queryHostMemTotal, Samples: totalVec},
		{Query: queryHostMemAvailable, Samples: availableVec},
	}
	return sb.String(), metrics, queries, nil
}

// measureNodeAllocatable reports allocatable CPU and memory per node.
func (s *ResourceFootprintSuite) measureNodeAllocatable(ctx context.Context) (string, []*pkgsuites.MetricResult, []string, error) {
	var sb strings.Builder
	queries := []string{queryAllocCPU, queryAllocMem}

	cpuVec, cpuWarn, err := pkgprom.RunInstant(ctx, s.Clients.PromClient, queryAllocCPU)
	s.addWarnings(&sb, queryAllocCPU, cpuWarn)
	if err != nil {
		return sb.String(), nil, queries, err
	}
	memVec, memWarn, err := pkgprom.RunInstant(ctx, s.Clients.PromClient, queryAllocMem)
	s.addWarnings(&sb, queryAllocMem, memWarn)
	if err != nil {
		return sb.String(), nil, queries, err
	}

	memByNode := make(map[string]float64, len(memVec))
	for _, sample := range memVec {
		memByNode[string(sample.Metric["node"])] = float64(sample.Value)
	}
	sort.Slice(cpuVec, func(i, j int) bool {
		return string(cpuVec[i].Metric["node"]) < string(cpuVec[j].Metric["node"])
	})

	tw := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NODE\tCPU ALLOCATABLE\tMEMORY ALLOCATABLE\t")
	fmt.Fprintln(tw, strings.Repeat("-", 4)+"\t"+strings.Repeat("-", 15)+"\t"+strings.Repeat("-", 18)+"\t")
	for _, sample := range cpuVec {
		node := string(sample.Metric["node"])
		fmt.Fprintf(tw, "%s\t%s\t%s\t\n", node, pkgprom.FormatMilliCPU(float64(sample.Value)), pkgprom.FormatMiB(memByNode[node]))
	}
	tw.Flush()

	metrics := []*pkgsuites.MetricResult{
		{Query: queryAllocCPU, Samples: cpuVec},
		{Query: queryAllocMem, Samples: memVec},
	}
	return sb.String(), metrics, queries, nil
}

func (s *ResourceFootprintSuite) RunE(ctx context.Context, runID, namespace string, opts pkgsuites.Options) (pkgsuites.SuiteResult, error) {
	cases := []struct {
		name    string
		measure func() (string, []*pkgsuites.MetricResult, []string, error)
	}{
		{
			name:    "per-namespace resource usage",
			measure: func() (string, []*pkgsuites.MetricResult, []string, error) { return s.measureNamespaceUsage(ctx) },
		},
		{
			name:    "node resource requests",
			measure: func() (string, []*pkgsuites.MetricResult, []string, error) { return s.measureNodeResourceRequests(ctx) },
		},
		{
			name:    "host memory",
			measure: func() (string, []*pkgsuites.MetricResult, []string, error) { return s.measureHostMemory(ctx) },
		},
		{
			name:    "node allocatable resources",
			measure: func() (string, []*pkgsuites.MetricResult, []string, error) { return s.measureNodeAllocatable(ctx) },
		},
	}

	var results []*pkgsuites.CaseResult
	for _, c := range cases {
		start := time.Now()
		out, metrics, queries, err := c.measure()
		cmdResults := make([]*pkgsuites.CmdResult, len(queries))
		for i, q := range queries {
			cmdResults[i] = &pkgsuites.CmdResult{Cmd: "promql " + q, Stdout: out}
		}
		if err != nil && len(cmdResults) > 0 {
			cmdResults[len(cmdResults)-1].Stderr = err.Error()
		}
		results = append(results, &pkgsuites.CaseResult{
			CaseName:      c.name,
			CmdResults:    cmdResults,
			MetricResults: metrics,
			DateTimeStart: start,
			DateTimeEnd:   time.Now(),
			Success:       err == nil,
		})
	}

	return pkgsuites.SuiteResult{
		Name:    s.Name(),
		RunID:   runID,
		Results: results,
	}, nil
}
