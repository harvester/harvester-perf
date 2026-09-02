package resourcefootprint

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	pkgprom "github.com/harvester/hvperf/pkg/prometheus"
	pkgsuites "github.com/harvester/hvperf/pkg/suites"
	"github.com/prometheus/common/model"
)

const (
	queryNodeCPU = `avg_over_time(sum by (node) (rate(container_cpu_usage_seconds_total{container!="", pod!=""}[5m]))[5m:10s])`
	queryNodeMem = `avg_over_time(sum by (node) (container_memory_working_set_bytes{container!="", pod!=""})[5m:10s])`

	queryNsCPU = `avg_over_time(sum by (namespace) (rate(container_cpu_usage_seconds_total{container!="", pod!=""}[5m]))[5m:10s])`
	queryNsMem = `avg_over_time(sum by (namespace) (container_memory_working_set_bytes{container!="", pod!=""})[5m:10s])`

	queryHostMemTotal       = `node_memory_MemTotal_bytes`
	queryHostMemAvailable   = `node_memory_MemAvailable_bytes`
	queryHostMemReclaimable = `node_memory_SReclaimable_bytes`
	queryHostMemConsumed    = `node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes`

	querySchedCPUReq = `avg_over_time(sum by (node) (kube_pod_container_resource_requests{resource="cpu"})[5m:10s])`
	querySchedMemReq = `avg_over_time(sum by (node) (kube_pod_container_resource_requests{resource="memory"})[5m:10s])`
	queryAllocCPU    = `kube_node_status_allocatable{resource="cpu"}`
	queryAllocMem    = `kube_node_status_allocatable{resource="memory"}`
)

type NodeUsageRow struct {
	Node   string `json:"node"`
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

type NamespaceUsageRow struct {
	Namespace string `json:"namespace"`
	CPU       string `json:"cpu"`
	Memory    string `json:"memory"`
}

type HostMemoryRow struct {
	Node         string `json:"node"`
	MemTotal     string `json:"memTotal"`
	MemAvailable string `json:"memAvailable"`
	SReclaimable string `json:"sReclaimable"`
	Consumed     string `json:"consumed"`
}

type SchedulerAllocationRow struct {
	Node   string `json:"node"`
	CPUReq string `json:"cpuReq"`
	CPUPct string `json:"cpuPct"`
	MemReq string `json:"memReq"`
	MemPct string `json:"memPct"`
}

var _ pkgsuites.Suite = &ResourceFootprintSuite{}

func init() {
	suite := &ResourceFootprintSuite{}
	suite.Marshal = suite
	pkgsuites.Register(suite)
}

// ResourceFootprintSuite implements test suite to assess node resource capacity.
type ResourceFootprintSuite struct {
	pkgsuites.SuiteMarshaler
	*pkgsuites.Clients
}

func (s *ResourceFootprintSuite) Name() string {
	return "resource-footprint"
}

func (s *ResourceFootprintSuite) Description() string {
	return "measure the resource footprint of the cluster"
}

func (s *ResourceFootprintSuite) IsReadWrite() bool {
	return false
}

func (s *ResourceFootprintSuite) SetClients(clients *pkgsuites.Clients) {
	s.Clients = clients
}

func (s *ResourceFootprintSuite) measureNodeUsage(ctx context.Context) (string, any, []string, error) {
	queries := []string{queryNodeCPU, queryNodeMem}
	cpuVec, err := pkgprom.RunInstant(ctx, s.Clients.PrometheusClient, queryNodeCPU)
	if err != nil {
		return "", nil, queries, err
	}
	memVec, err := pkgprom.RunInstant(ctx, s.Clients.PrometheusClient, queryNodeMem)
	if err != nil {
		return "", nil, queries, err
	}

	cpuByNode := pkgprom.VectorByLabel(cpuVec, "node")
	memByNode := pkgprom.VectorByLabel(memVec, "node")

	var rows []NodeUsageRow
	var sb strings.Builder
	fmt.Fprintf(&sb, "%-30s  %10s  %12s\n", "NODE", "CPU(cores)", "MEMORY(bytes)")
	for _, node := range labelUnion(cpuByNode, memByNode) {
		cpu := pkgprom.MilliCores(float64(cpuByNode[node]))
		mem := pkgprom.Mebibytes(float64(memByNode[node]))
		rows = append(rows, NodeUsageRow{Node: node, CPU: cpu, Memory: mem})
		fmt.Fprintf(&sb, "%-30s  %10s  %12s\n", node, cpu, mem)
	}
	return sb.String(), rows, queries, nil
}

func (s *ResourceFootprintSuite) measureNamespaceUsage(ctx context.Context) (string, any, []string, error) {
	queries := []string{queryNsCPU, queryNsMem}
	cpuVec, err := pkgprom.RunInstant(ctx, s.Clients.PrometheusClient, queryNsCPU)
	if err != nil {
		return "", nil, queries, err
	}
	memVec, err := pkgprom.RunInstant(ctx, s.Clients.PrometheusClient, queryNsMem)
	if err != nil {
		return "", nil, queries, err
	}

	cpuByNs := pkgprom.VectorByLabel(cpuVec, "namespace")
	memByNs := pkgprom.VectorByLabel(memVec, "namespace")

	var rows []NamespaceUsageRow
	var sb strings.Builder
	fmt.Fprintf(&sb, "%-45s  %10s  %12s\n", "NAMESPACE", "CPU(cores)", "MEMORY(bytes)")
	for _, ns := range labelUnion(cpuByNs, memByNs) {
		cpu := pkgprom.MilliCores(float64(cpuByNs[ns]))
		mem := pkgprom.Mebibytes(float64(memByNs[ns]))
		rows = append(rows, NamespaceUsageRow{Namespace: ns, CPU: cpu, Memory: mem})
		fmt.Fprintf(&sb, "%-45s  %10s  %12s\n", ns, cpu, mem)
	}
	return sb.String(), rows, queries, nil
}

func (s *ResourceFootprintSuite) measureHostMemory(ctx context.Context) (string, any, []string, error) {
	queries := []string{queryHostMemTotal, queryHostMemAvailable, queryHostMemReclaimable, queryHostMemConsumed}

	type memRow struct {
		total, available, reclaimable, consumed float64
	}
	rows := map[string]*memRow{}

	fetch := []struct {
		q   string
		set func(r *memRow, v float64)
	}{
		{queryHostMemTotal, func(r *memRow, v float64) { r.total = v }},
		{queryHostMemAvailable, func(r *memRow, v float64) { r.available = v }},
		{queryHostMemReclaimable, func(r *memRow, v float64) { r.reclaimable = v }},
		{queryHostMemConsumed, func(r *memRow, v float64) { r.consumed = v }},
	}

	for _, f := range fetch {
		vec, err := pkgprom.RunInstant(ctx, s.Clients.PrometheusClient, f.q)
		if err != nil {
			return "", nil, queries, err
		}
		for _, sample := range vec {
			node := string(sample.Metric["instance"])
			if rows[node] == nil {
				rows[node] = &memRow{}
			}
			f.set(rows[node], float64(sample.Value))
		}
	}

	nodes := make([]string, 0, len(rows))
	for n := range rows {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)

	var data []HostMemoryRow
	var sb strings.Builder
	fmt.Fprintf(&sb, "%-30s  %12s  %12s  %14s  %12s\n", "NODE", "MemTotal", "MemAvailable", "SReclaimable", "Consumed")
	for _, node := range nodes {
		r := rows[node]
		data = append(data, HostMemoryRow{
			Node:         node,
			MemTotal:     pkgprom.Gibibytes(r.total),
			MemAvailable: pkgprom.Gibibytes(r.available),
			SReclaimable: pkgprom.Gibibytes(r.reclaimable),
			Consumed:     pkgprom.Gibibytes(r.consumed),
		})
		fmt.Fprintf(&sb, "%-30s  %12s  %12s  %14s  %12s\n",
			node,
			pkgprom.Gibibytes(r.total),
			pkgprom.Gibibytes(r.available),
			pkgprom.Gibibytes(r.reclaimable),
			pkgprom.Gibibytes(r.consumed),
		)
	}
	return sb.String(), data, queries, nil
}

func (s *ResourceFootprintSuite) measureSchedulerAllocation(ctx context.Context) (string, any, []string, error) {
	queries := []string{querySchedCPUReq, querySchedMemReq, queryAllocCPU, queryAllocMem}
	cpuReqVec, err := pkgprom.RunInstant(ctx, s.Clients.PrometheusClient, querySchedCPUReq)
	if err != nil {
		return "", nil, queries, err
	}
	memReqVec, err := pkgprom.RunInstant(ctx, s.Clients.PrometheusClient, querySchedMemReq)
	if err != nil {
		return "", nil, queries, err
	}
	allocCPUVec, err := pkgprom.RunInstant(ctx, s.Clients.PrometheusClient, queryAllocCPU)
	if err != nil {
		return "", nil, queries, err
	}
	allocMemVec, err := pkgprom.RunInstant(ctx, s.Clients.PrometheusClient, queryAllocMem)
	if err != nil {
		return "", nil, queries, err
	}

	cpuReq := pkgprom.VectorByLabel(cpuReqVec, "node")
	memReq := pkgprom.VectorByLabel(memReqVec, "node")
	allocCPU := pkgprom.VectorByLabel(allocCPUVec, "node")
	allocMem := pkgprom.VectorByLabel(allocMemVec, "node")

	var data []SchedulerAllocationRow
	var sb strings.Builder
	fmt.Fprintf(&sb, "%-30s  %12s  %8s  %12s  %8s\n", "NODE", "CPU Req", "CPU%", "Mem Req", "Mem%")
	for _, node := range labelUnion(cpuReq, memReq) {
		cpuReqStr := pkgprom.MilliCores(float64(cpuReq[node]))
		cpuPct := pkgprom.Pct(float64(cpuReq[node]), float64(allocCPU[node]))
		memReqStr := pkgprom.Mebibytes(float64(memReq[node]))
		memPct := pkgprom.Pct(float64(memReq[node]), float64(allocMem[node]))
		data = append(data, SchedulerAllocationRow{
			Node:   node,
			CPUReq: cpuReqStr,
			CPUPct: cpuPct,
			MemReq: memReqStr,
			MemPct: memPct,
		})
		fmt.Fprintf(&sb, "%-30s  %12s  %7s%%  %12s  %7s%%\n", node, cpuReqStr, cpuPct, memReqStr, memPct)
	}
	return sb.String(), data, queries, nil
}

func (s *ResourceFootprintSuite) RunE(ctx context.Context, runID, namespace string, opts pkgsuites.Options) (pkgsuites.SuiteResult, error) {
	cases := []struct {
		name    string
		measure func() (string, any, []string, error)
	}{
		{
			name:    "node cpu and memory usage",
			measure: func() (string, any, []string, error) { return s.measureNodeUsage(ctx) },
		},
		{
			name:    "per-namespace cpu and memory usage",
			measure: func() (string, any, []string, error) { return s.measureNamespaceUsage(ctx) },
		},
		{
			name:    "host memory",
			measure: func() (string, any, []string, error) { return s.measureHostMemory(ctx) },
		},
		{
			name:    "scheduler allocation",
			measure: func() (string, any, []string, error) { return s.measureSchedulerAllocation(ctx) },
		},
	}

	var results []*pkgsuites.CaseResult
	for _, c := range cases {
		start := time.Now()
		out, data, queries, err := c.measure()
		results = append(results, &pkgsuites.CaseResult{
			CaseName:      c.name,
			Cmds:          queryCmds(queries),
			DateTimeStart: start,
			DateTimeEnd:   time.Now(),
			Success:       err == nil,
			Out:           out,
			Data:          data,
			Err:           err,
		})
	}

	return pkgsuites.SuiteResult{
		Name:    s.Name(),
		RunID:   runID,
		Results: results,
	}, nil
}

func queryCmds(queries []string) [][]string {
	cmds := make([][]string, len(queries))
	for i, q := range queries {
		cmds[i] = []string{"promql", q}
	}
	return cmds
}

// Defensive programming, ensure cpu/mem co-exisit for same target.
func labelUnion(a, b map[string]model.SampleValue) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		seen[k] = struct{}{}
	}
	for k := range b {
		seen[k] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
