package resourcefootprint

// As PoC for now,  measure 5 mins,  with 10 seconds  step.

//
// func runQuery(ctx context.Context, client promv1.API, query string) (model.Vector, error) {
// 	result, warnings, err := client.Query(ctx, query, time.Now())
// 	if err != nil {
// 		return nil, fmt.Errorf("query %q: %w", query, err)
// 	}
// 	for _, w := range warnings {
// 		fmt.Printf("warning: %s\n", w)
// 	}
// 	v, ok := result.(model.Vector)
// 	if !ok {
// 		return nil, fmt.Errorf("query %q: expected Vector, got %T", query, result)
// 	}
// 	return v, nil
// }

// measureNodeUsage returns a formatted string of per-node CPU and memory usage.
// // measureNamespaceUsage returns a formatted string of per-namespace CPU and memory usage.
// func measureNamespaceUsage(ctx context.Context, client promv1.API) (string, error) {
// 	cpuVec, err := runQuery(ctx, client, queryNsCPU)
// 	if err != nil {
// 		return "", err
// 	}
// 	memVec, err := runQuery(ctx, client, queryNsMem)
// 	if err != nil {
// 		return "", err
// 	}
//
// 	cpuByNs := vectorByLabel(cpuVec, "namespace")
// 	memByNs := vectorByLabel(memVec, "namespace")
//
// 	namespaces := labelUnion(cpuByNs, memByNs)
// 	var sb strings.Builder
// 	fmt.Fprintf(&sb, "%-45s  %10s  %12s\n", "NAMESPACE", "CPU(cores)", "MEMORY(bytes)")
// 	for _, ns := range namespaces {
// 		cpu := milliCores(float64(cpuByNs[ns]))
// 		mem := mebibytes(float64(memByNs[ns]))
// 		fmt.Fprintf(&sb, "%-45s  %10s  %12s\n", ns, cpu, mem)
// 	}
// 	return sb.String(), nil
// }
//
// // measureHostMemory returns a formatted string of host-level memory stats per node.
// func measureHostMemory(ctx context.Context, client promv1.API) (string, error) {
// 	type memRow struct {
// 		total, available, reclaimable, consumed float64
// 	}
// 	rows := map[string]*memRow{}
//
// 	queries := []struct {
// 		q   string
// 		set func(r *memRow, v float64)
// 	}{
// 		{queryHostMemTotal, func(r *memRow, v float64) { r.total = v }},
// 		{queryHostMemAvailable, func(r *memRow, v float64) { r.available = v }},
// 		{queryHostMemReclaimable, func(r *memRow, v float64) { r.reclaimable = v }},
// 		{queryHostMemConsumed, func(r *memRow, v float64) { r.consumed = v }},
// 	}
//
// 	for _, q := range queries {
// 		vec, err := runQuery(ctx, client, q.q)
// 		if err != nil {
// 			return "", err
// 		}
// 		for _, sample := range vec {
// 			node := string(sample.Metric["instance"])
// 			if rows[node] == nil {
// 				rows[node] = &memRow{}
// 			}
// 			q.set(rows[node], float64(sample.Value))
// 		}
// 	}
//
// 	slices.Sort(rows)
// 	var sb strings.Builder
// 	fmt.Fprintf(&sb, "%-30s  %12s  %12s  %14s  %12s\n",
// 		"NODE", "MemTotal", "MemAvailable", "SReclaimable", "Consumed")
// 	for _, node := range rows {
// 		r := rows[node]
// 		fmt.Fprintf(&sb, "%-30s  %12s  %12s  %14s  %12s\n",
// 			node,
// 			gibibytes(r.total),
// 			gibibytes(r.available),
// 			gibibytes(r.reclaimable),
// 			gibibytes(r.consumed),
// 		)
// 	}
// 	return sb.String(), nil
// }
//
// // measureSchedulerAllocation returns a formatted string of scheduler CPU and memory requests per node.
// func measureSchedulerAllocation(ctx context.Context, client promv1.API) (string, error) {
// 	cpuReqVec, err := runQuery(ctx, client, querySchedCPUReq)
// 	if err != nil {
// 		return "", err
// 	}
// 	memReqVec, err := runQuery(ctx, client, querySchedMemReq)
// 	if err != nil {
// 		return "", err
// 	}
// 	allocCPUVec, err := runQuery(ctx, client, queryAllocCPU)
// 	if err != nil {
// 		return "", err
// 	}
// 	allocMemVec, err := runQuery(ctx, client, queryAllocMem)
// 	if err != nil {
// 		return "", err
// 	}
//
// 	cpuReq := vectorByLabel(cpuReqVec, "node")
// 	memReq := vectorByLabel(memReqVec, "node")
// 	allocCPU := vectorByLabel(allocCPUVec, "node")
// 	allocMem := vectorByLabel(allocMemVec, "node")
//
// 	nodes := labelUnion(cpuReq, memReq)
// 	var sb strings.Builder
// 	fmt.Fprintf(&sb, "%-30s  %12s  %8s  %12s  %8s\n",
// 		"NODE", "CPU Req", "CPU%", "Mem Req", "Mem%")
// 	for _, node := range nodes {
// 		cpuPct := pct(float64(cpuReq[node]), float64(allocCPU[node]))
// 		memPct := pct(float64(memReq[node]), float64(allocMem[node]))
// 		fmt.Fprintf(&sb, "%-30s  %12s  %7s%%  %12s  %7s%%\n",
// 			node,
// 			milliCores(float64(cpuReq[node])),
// 			cpuPct,
// 			mebibytes(float64(memReq[node])),
// 			memPct,
// 		)
// 	}
// 	return sb.String(), nil
// }
//
// // --- helpers ---
