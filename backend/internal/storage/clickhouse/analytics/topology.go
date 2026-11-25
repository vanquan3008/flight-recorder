package analytics

import (
	"context"
	"sort"
	"strings"

	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"
)

type topologyRow struct {
	SourceName          string   `json:"source_name"`
	TargetType          string   `json:"target_type"`
	TargetName          string   `json:"target_name"`
	EdgeKind            string   `json:"edge_kind"`
	CallCount           int64    `json:"call_count"`
	ErrorCount          int64    `json:"error_count"`
	AverageLatencyNanos int64    `json:"average_latency_nanos"`
	P95LatencyNanos     int64    `json:"p95_latency_nanos"`
	TraceIDs            []string `json:"trace_ids"`
}

// ListTopology returns service-to-service edges, from a child span to a parent
// in another service, and edges from services to the dependencies they call.
func (store *Analytics) ListTopology(ctx context.Context, projectID string, filter analysis.Filter) (analysis.Topology, error) {
	where, parameters, err := client.Scope(projectID, filter.EnvironmentID)
	if err != nil {
		return analysis.Topology{}, err
	}
	filter.From = client.MaxTime(filter.From, filter.Since)
	window := client.AddTime(append([]string{}, where...), parameters, "started_at", "from", ">=", filter.From)
	window = client.AddTime(window, parameters, "started_at", "to", "<", filter.To)
	parents := client.AddTime(append([]string{}, where...), parameters, "started_at", "parent_from", ">=", client.MaxTime(parentStart(filter.From), filter.Since))
	parents = client.AddTime(parents, parameters, "started_at", "to", "<", filter.To)
	edgeFilter := ""
	if filter.Service != "" {
		edgeFilter = `
WHERE source_name = {service:String}
   OR (target_type = 'service' AND target_name = {service:String})`
		parameters["service"] = filter.Service
	}
	rows, err := client.QueryRows[topologyRow](ctx, store.client, `SELECT
    source_name,
    target_type,
    target_name,
    edge_kind,
    count() AS call_count,
    countIf(status_code = 'ERROR') AS error_count,
    toInt64(avg(duration_ns)) AS average_latency_nanos,
    toInt64(quantileTDigest(0.95)(duration_ns)) AS p95_latency_nanos,
    arraySlice(arraySort(groupUniqArray(trace_id)), 1, 5) AS trace_ids
FROM (
    SELECT
        parent.service_name AS source_name,
        'service' AS target_type,
        child.service_name AS target_name,
        'service' AS edge_kind,
        child.trace_id AS trace_id,
        child.status_code AS status_code,
        child.duration_ns AS duration_ns
    FROM (
        SELECT trace_id, parent_span_id, service_name, status_code, duration_ns
        FROM tracepilot_spans FINAL
        WHERE `+strings.Join(window, " AND ")+`
          AND parent_span_id != ''
    ) AS child
    INNER JOIN (
        SELECT trace_id, span_id, service_name
        FROM tracepilot_spans FINAL
        WHERE `+strings.Join(parents, " AND ")+`
    ) AS parent
        ON child.trace_id = parent.trace_id
       AND child.parent_span_id = parent.span_id
    WHERE parent.service_name != child.service_name

    UNION ALL

    SELECT service_name, dependency_type, dependency_name, dependency_type, trace_id, status_code, duration_ns
    FROM tracepilot_spans FINAL
    WHERE `+strings.Join(window, " AND ")+`
      AND dependency_type != ''
      AND dependency_name != ''
)`+edgeFilter+`
GROUP BY source_name, target_type, target_name, edge_kind
ORDER BY source_name, target_type, target_name`+client.FinalSettings+`
FORMAT JSONEachRow`, parameters)
	if err != nil {
		return analysis.Topology{}, err
	}
	edges := make([]analysis.TopologyEdge, 0, len(rows))
	nodes := make(map[string]analysis.TopologyNode)
	for _, row := range rows {
		edge := analysis.TopologyEdge{
			SourceNodeID: analysis.EntityID("service", row.SourceName), TargetNodeID: analysis.EntityID(row.TargetType, row.TargetName),
			Kind: row.EdgeKind, CallCount: row.CallCount, ErrorCount: row.ErrorCount,
			AverageLatencyNano: row.AverageLatencyNanos, P95LatencyNano: row.P95LatencyNanos, TraceIDs: row.TraceIDs,
		}
		nodes[edge.SourceNodeID] = analysis.TopologyNode{ID: edge.SourceNodeID, Type: "service", Name: row.SourceName}
		nodes[edge.TargetNodeID] = analysis.TopologyNode{ID: edge.TargetNodeID, Type: row.TargetType, Name: row.TargetName}
		edges = append(edges, edge)
	}
	// A service need not have an outgoing edge to appear in the topology.
	services, err := store.ListServices(ctx, projectID, filter.EnvironmentID, filter.From, filter.To)
	if err != nil {
		return analysis.Topology{}, err
	}
	for _, service := range services {
		if filter.Service == "" || filter.Service == service.Name {
			id := analysis.EntityID("service", service.Name)
			nodes[id] = analysis.TopologyNode{ID: id, Type: "service", Name: service.Name}
		}
	}
	nodeList := make([]analysis.TopologyNode, 0, len(nodes))
	for _, node := range nodes {
		nodeList = append(nodeList, node)
	}
	sort.Slice(nodeList, func(i, j int) bool { return nodeList[i].ID < nodeList[j].ID })
	return analysis.Topology{From: filter.From, To: filter.To, Nodes: nodeList, Edges: edges}, nil
}
