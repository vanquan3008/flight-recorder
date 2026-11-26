package analytics

import (
	"context"
	"fmt"
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
	statement := fmt.Sprintf(listTopologySQL,
		strings.Join(window, " AND "), strings.Join(parents, " AND "),
		strings.Join(window, " AND "), edgeFilter, client.FinalSettings,
	)
	rows, err := client.QueryRows[topologyRow](ctx, store.client, statement, parameters)
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
