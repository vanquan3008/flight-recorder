package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
)

func (database *Database) ListTopology(ctx context.Context, projectID string, filter analysis.Filter) (analysis.Topology, error) {
	rows, err := database.pool.Query(ctx, `
		WITH window_spans AS (
			SELECT * FROM spans
			WHERE project_id=$1 AND environment_id=$2 AND started_at >= $3 AND started_at < $4
		), raw_edges AS (
			SELECT parent.service_name source_name, 'service' target_type, child.service_name target_name,
				'service' edge_kind, child.trace_id, child.status_code,
				extract(epoch FROM child.ended_at-child.started_at)*1000000000 duration_ns
			FROM window_spans child
			JOIN spans parent ON parent.project_id=child.project_id AND parent.environment_id=child.environment_id
			 AND parent.trace_id=child.trace_id AND parent.span_id=child.parent_span_id
			WHERE parent.service_name <> child.service_name
			UNION ALL
			SELECT service_name,
				`+dependencyTypeSQL+`, `+dependencyNameSQL+`, `+dependencyTypeSQL+`,
				trace_id, status_code, extract(epoch FROM ended_at-started_at)*1000000000
			FROM window_spans
			WHERE `+dependencyPredicateSQL+`
		)
		SELECT source_name,target_type,target_name,edge_kind,count(*),
			count(*) FILTER (WHERE status_code='ERROR'),
			COALESCE(avg(duration_ns),0)::bigint,
			COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ns),0)::bigint,
			(array_agg(DISTINCT trace_id ORDER BY trace_id))[1:5]
		FROM raw_edges
		WHERE target_name IS NOT NULL AND target_name <> '' AND ($5='' OR source_name=$5 OR (target_type='service' AND target_name=$5))
		GROUP BY source_name,target_type,target_name,edge_kind
		ORDER BY source_name,target_type,target_name`, projectID, filter.EnvironmentID, filter.From, filter.To, filter.Service)
	if err != nil {
		return analysis.Topology{}, fmt.Errorf("list topology: %w", err)
	}
	defer rows.Close()
	edges := make([]analysis.TopologyEdge, 0)
	nodes := make(map[string]analysis.TopologyNode)
	for rows.Next() {
		var sourceName, targetType, targetName string
		var edge analysis.TopologyEdge
		if err := rows.Scan(&sourceName, &targetType, &targetName, &edge.Kind, &edge.CallCount, &edge.ErrorCount, &edge.AverageLatencyNano, &edge.P95LatencyNano, &edge.TraceIDs); err != nil {
			return analysis.Topology{}, fmt.Errorf("scan topology: %w", err)
		}
		edge.SourceNodeID = entityID("service", sourceName)
		edge.TargetNodeID = entityID(targetType, targetName)
		nodes[edge.SourceNodeID] = analysis.TopologyNode{ID: edge.SourceNodeID, Type: "service", Name: sourceName}
		nodes[edge.TargetNodeID] = analysis.TopologyNode{ID: edge.TargetNodeID, Type: targetType, Name: targetName}
		edges = append(edges, edge)
	}
	if err := rows.Err(); err != nil {
		return analysis.Topology{}, fmt.Errorf("iterate topology: %w", err)
	}
	rows.Close()
	// A service need not have an outgoing edge to appear in the topology.
	services, err := database.ListServices(ctx, projectID, filter.EnvironmentID, filter.From, filter.To)
	if err != nil {
		return analysis.Topology{}, err
	}
	for _, service := range services {
		if filter.Service == "" || filter.Service == service.Name {
			id := entityID("service", service.Name)
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

func (database *Database) ListMetricPoints(ctx context.Context, projectID string, filter analysis.Filter) ([]analysis.MetricPoint, error) {
	rows, err := database.pool.Query(ctx, `
		SELECT id,metric_name,metric_unit,metric_type,service_name,occurred_at,value,hist_count,hist_sum,attributes,exemplars
		FROM metric_data_points
		WHERE project_id=$1 AND environment_id=$2 AND occurred_at >= $3 AND occurred_at <= $4
		  AND ($5='' OR service_name=$5) AND ($6='' OR metric_name=$6)
		ORDER BY occurred_at DESC,id DESC LIMIT $7`, projectID, filter.EnvironmentID, filter.From, filter.To, filter.Service, filter.Metric, filter.Limit)
	if err != nil {
		return nil, fmt.Errorf("list metric points: %w", err)
	}
	defer rows.Close()
	points := make([]analysis.MetricPoint, 0)
	for rows.Next() {
		var point analysis.MetricPoint
		var attributes, exemplars []byte
		if err := rows.Scan(&point.ID, &point.Name, &point.Unit, &point.Type, &point.Service, &point.OccurredAt, &point.Value, &point.Count, &point.Sum, &attributes, &exemplars); err != nil {
			return nil, fmt.Errorf("scan metric point: %w", err)
		}
		if err := json.Unmarshal(attributes, &point.Attributes); err != nil {
			return nil, fmt.Errorf("decode metric attributes: %w", err)
		}
		if err := json.Unmarshal(exemplars, &point.Exemplars); err != nil {
			return nil, fmt.Errorf("decode metric exemplars: %w", err)
		}
		points = append(points, point)
	}
	return points, rows.Err()
}

func (database *Database) TelemetryQuality(ctx context.Context, projectID string, filter analysis.Filter) (analysis.Quality, error) {
	quality := analysis.Quality{From: filter.From, To: filter.To}
	err := database.pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM spans s WHERE s.project_id=$1 AND s.environment_id=$2 AND s.started_at BETWEEN $3 AND $4 AND ($5='' OR s.service_name=$5)),
			(SELECT count(*) FROM spans s WHERE s.project_id=$1 AND s.environment_id=$2 AND s.started_at BETWEEN $3 AND $4 AND s.parent_span_id IS NOT NULL
			 AND ($5='' OR s.service_name=$5)
			 AND NOT EXISTS (SELECT 1 FROM spans p WHERE p.project_id=s.project_id AND p.environment_id=s.environment_id AND p.trace_id=s.trace_id AND p.span_id=s.parent_span_id)),
			(SELECT count(*) FROM logs l WHERE l.project_id=$1 AND l.environment_id=$2 AND l.occurred_at BETWEEN $3 AND $4 AND ($5='' OR l.service_name=$5)),
			(SELECT count(*) FROM logs l WHERE l.project_id=$1 AND l.environment_id=$2 AND l.occurred_at BETWEEN $3 AND $4 AND l.trace_id IS NULL AND ($5='' OR l.service_name=$5)),
			(SELECT count(*) FROM metric_data_points m WHERE m.project_id=$1 AND m.environment_id=$2 AND m.occurred_at BETWEEN $3 AND $4 AND ($5='' OR m.service_name=$5))`,
		projectID, filter.EnvironmentID, filter.From, filter.To, filter.Service).Scan(&quality.Spans, &quality.OrphanSpans, &quality.Logs, &quality.UncorrelatedLogs, &quality.MetricPoints)
	if err != nil {
		return analysis.Quality{}, fmt.Errorf("telemetry quality: %w", err)
	}
	return quality, nil
}

func entityID(kind, name string) string { return kind + ":" + name }
