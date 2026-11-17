package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

// parentLookback is how far before a window a parent span may have started and
// still be matched to a child inside it. Bounding the lookup keeps it from
// probing every daily partition; see the note on ListTopologyEdges.
const parentLookback = time.Hour

func (store *AnalyticsStore) ListTopology(ctx context.Context, projectID string, filter analysis.Filter) (analysis.Topology, error) {
	rows, err := store.queries.ListTopologyEdges(ctx, sqlcstore.ListTopologyEdgesParams{
		ProjectID: projectID, EnvironmentID: filter.EnvironmentID,
		FromTime: filter.From, ToTime: filter.To, Service: pgvalue.OrNil(filter.Service),
		ParentWindowStart: filter.From.Add(-parentLookback),
	})
	if err != nil {
		return analysis.Topology{}, fmt.Errorf("list topology: %w", err)
	}
	edges := make([]analysis.TopologyEdge, 0, len(rows))
	nodes := make(map[string]analysis.TopologyNode)
	for _, row := range rows {
		edge := analysis.TopologyEdge{
			SourceNodeID: pgvalue.EntityID("service", row.SourceName),
			TargetNodeID: pgvalue.EntityID(row.TargetType, row.TargetName),
			Kind:         row.EdgeKind, CallCount: row.CallCount, ErrorCount: row.ErrorCount,
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
			id := pgvalue.EntityID("service", service.Name)
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

func (store *AnalyticsStore) ListMetricPoints(ctx context.Context, projectID string, filter analysis.Filter) ([]analysis.MetricPoint, error) {
	rows, err := store.queries.ListMetricPoints(ctx, sqlcstore.ListMetricPointsParams{
		ProjectID: projectID, EnvironmentID: filter.EnvironmentID, FromTime: filter.From, ToTime: filter.To,
		Service: pgvalue.OrNil(filter.Service), Metric: pgvalue.OrNil(filter.Metric), RowLimit: int32(filter.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list metric points: %w", err)
	}
	points := make([]analysis.MetricPoint, 0, len(rows))
	for _, row := range rows {
		point := analysis.MetricPoint{
			ID: row.ID, Name: row.MetricName, Unit: row.MetricUnit, Type: row.MetricType, Service: row.ServiceName,
			OccurredAt: row.OccurredAt, Value: row.Value, Count: row.HistCount, Sum: row.HistSum,
		}
		if err := json.Unmarshal(row.Attributes, &point.Attributes); err != nil {
			return nil, fmt.Errorf("decode metric attributes: %w", err)
		}
		if err := json.Unmarshal(row.Exemplars, &point.Exemplars); err != nil {
			return nil, fmt.Errorf("decode metric exemplars: %w", err)
		}
		points = append(points, point)
	}
	return points, nil
}

func (store *AnalyticsStore) TelemetryQuality(ctx context.Context, projectID string, filter analysis.Filter) (analysis.Quality, error) {
	row, err := store.queries.TelemetryQuality(ctx, sqlcstore.TelemetryQualityParams{
		ProjectID: projectID, EnvironmentID: filter.EnvironmentID, FromTime: filter.From, ToTime: filter.To,
		Service: pgvalue.OrNil(filter.Service), ParentWindowStart: filter.From.Add(-parentLookback),
	})
	if err != nil {
		return analysis.Quality{}, fmt.Errorf("telemetry quality: %w", err)
	}
	// Log counts are filled from ClickHouse by the caller; PostgreSQL stores no logs.
	return analysis.Quality{
		From: filter.From, To: filter.To, Spans: row.Spans, OrphanSpans: row.OrphanSpans, MetricPoints: row.MetricPoints,
	}, nil
}
