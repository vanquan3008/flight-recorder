package clickhouse

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/query"
)

// parentLookback is how far before a window a parent span may have started and
// still be matched to a child inside it.
const parentLookback = time.Hour

// dependencyWindow bounds the service dependency list, which takes no window.
const dependencyWindow = 24 * time.Hour

func entityID(kind, name string) string { return kind + ":" + name }

func parentStart(from time.Time) time.Time {
	if from.IsZero() {
		return from
	}
	return from.Add(-parentLookback)
}

type serviceRow struct {
	Name           string  `json:"name"`
	TraceCount     int64   `json:"trace_count"`
	RequestCount   int64   `json:"request_count"`
	ErrorCount     int64   `json:"error_count"`
	ErrorRate      float64 `json:"error_rate"`
	AverageLatency int64   `json:"average_latency"`
}

func (store *Store) ListServices(ctx context.Context, projectID, environmentID string, from, to time.Time) ([]query.ServiceSummary, error) {
	where, parameters, err := store.scope(projectID, environmentID)
	if err != nil {
		return nil, err
	}
	where = addTime(where, parameters, "started_at", "from", ">=", from)
	where = addTime(where, parameters, "started_at", "to", "<=", to)
	rows, err := queryRows[serviceRow](ctx, store, `SELECT service_name AS name, uniqExact(trace_id) AS trace_count,
countIf(span_kind = 2) AS request_count, countIf(status_code = 'ERROR') AS error_count,
if(count() = 0, 0, error_count / count()) AS error_rate, toInt64(avg(duration_ns)) AS average_latency
FROM tracepilot_spans FINAL WHERE `+strings.Join(where, " AND ")+" GROUP BY service_name ORDER BY service_name"+finalSettings+" FORMAT JSONEachRow", parameters)
	if err != nil {
		return nil, err
	}
	services := make([]query.ServiceSummary, 0, len(rows))
	for _, row := range rows {
		services = append(services, query.ServiceSummary(row))
	}
	return services, nil
}

type dependencyRow struct {
	ServiceName string `json:"service_name"`
	CallCount   int64  `json:"call_count"`
}

// ListServiceDependencies counts calls from a service to other services over the last day.
func (store *Store) ListServiceDependencies(ctx context.Context, projectID, environmentID, serviceName string) ([]query.ServiceDependency, error) {
	_, parameters, err := store.scope(projectID, environmentID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	parameters["service"], parameters["from"], parameters["to"] = serviceName, chTime(now.Add(-dependencyWindow)), chTime(now)
	rows, err := queryRows[dependencyRow](ctx, store, `SELECT child.service_name AS service_name, count() AS call_count
FROM (SELECT trace_id, parent_span_id, service_name FROM tracepilot_spans FINAL WHERE project_id = {project:UUID}
  AND environment_id = {environment:UUID} AND started_at >= {from:DateTime64(9, 'UTC')} AND started_at <= {to:DateTime64(9, 'UTC')}
  AND parent_span_id != '' AND service_name != {service:String}) AS child
INNER JOIN (SELECT trace_id, span_id FROM tracepilot_spans FINAL WHERE project_id = {project:UUID} AND environment_id = {environment:UUID}
  AND started_at >= {from:DateTime64(9, 'UTC')} - INTERVAL 1 HOUR AND started_at <= {to:DateTime64(9, 'UTC')}
  AND service_name = {service:String}) AS parent ON child.trace_id = parent.trace_id AND child.parent_span_id = parent.span_id
GROUP BY service_name ORDER BY call_count DESC, service_name`+finalSettings+" FORMAT JSONEachRow", parameters)
	if err != nil {
		return nil, err
	}
	dependencies := make([]query.ServiceDependency, 0, len(rows))
	for _, row := range rows {
		dependencies = append(dependencies, query.ServiceDependency(row))
	}
	return dependencies, nil
}

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
func (store *Store) ListTopology(ctx context.Context, projectID string, filter analysis.Filter) (analysis.Topology, error) {
	where, parameters, err := store.scope(projectID, filter.EnvironmentID)
	if err != nil {
		return analysis.Topology{}, err
	}
	window := addTime(append([]string{}, where...), parameters, "started_at", "from", ">=", filter.From)
	window = addTime(window, parameters, "started_at", "to", "<", filter.To)
	parents := addTime(append([]string{}, where...), parameters, "started_at", "parent_from", ">=", parentStart(filter.From))
	parents = addTime(parents, parameters, "started_at", "to", "<", filter.To)
	edgeFilter := ""
	if filter.Service != "" {
		edgeFilter = " WHERE source_name = {service:String} OR (target_type = 'service' AND target_name = {service:String})"
		parameters["service"] = filter.Service
	}
	rows, err := queryRows[topologyRow](ctx, store, `SELECT source_name, target_type, target_name, edge_kind, count() AS call_count,
countIf(status_code = 'ERROR') AS error_count, toInt64(avg(duration_ns)) AS average_latency_nanos,
toInt64(quantileTDigest(0.95)(duration_ns)) AS p95_latency_nanos, arraySlice(arraySort(groupUniqArray(trace_id)), 1, 5) AS trace_ids
FROM (
  SELECT parent.service_name AS source_name, 'service' AS target_type, child.service_name AS target_name, 'service' AS edge_kind,
    child.trace_id AS trace_id, child.status_code AS status_code, child.duration_ns AS duration_ns
  FROM (SELECT trace_id, parent_span_id, service_name, status_code, duration_ns FROM tracepilot_spans FINAL
    WHERE `+strings.Join(window, " AND ")+` AND parent_span_id != '') AS child
  INNER JOIN (SELECT trace_id, span_id, service_name FROM tracepilot_spans FINAL WHERE `+strings.Join(parents, " AND ")+`) AS parent
    ON child.trace_id = parent.trace_id AND child.parent_span_id = parent.span_id
  WHERE parent.service_name != child.service_name
  UNION ALL
  SELECT service_name, dependency_type, dependency_name, dependency_type, trace_id, status_code, duration_ns
  FROM tracepilot_spans FINAL WHERE `+strings.Join(window, " AND ")+` AND dependency_type != '' AND dependency_name != ''
)`+edgeFilter+` GROUP BY source_name, target_type, target_name, edge_kind ORDER BY source_name, target_type, target_name`+finalSettings+" FORMAT JSONEachRow", parameters)
	if err != nil {
		return analysis.Topology{}, err
	}
	edges := make([]analysis.TopologyEdge, 0, len(rows))
	nodes := make(map[string]analysis.TopologyNode)
	for _, row := range rows {
		edge := analysis.TopologyEdge{
			SourceNodeID: entityID("service", row.SourceName), TargetNodeID: entityID(row.TargetType, row.TargetName),
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

type metricPointRow struct {
	ID         string            `json:"id"`
	Name       string            `json:"metric_name"`
	Unit       string            `json:"metric_unit"`
	Type       string            `json:"metric_type"`
	Service    string            `json:"service_name"`
	OccurredAt time.Time         `json:"occurred_at"`
	Value      *float64          `json:"value"`
	Count      *int64            `json:"hist_count"`
	Sum        *float64          `json:"hist_sum"`
	Attributes map[string]string `json:"attributes"`
	Exemplars  string            `json:"exemplars"`
}

func (store *Store) ListMetricPoints(ctx context.Context, projectID string, filter analysis.Filter) ([]analysis.MetricPoint, error) {
	if filter.Limit < 1 {
		return []analysis.MetricPoint{}, nil
	}
	where, parameters, err := store.scope(projectID, filter.EnvironmentID)
	if err != nil {
		return nil, err
	}
	where = addTime(where, parameters, "occurred_at", "from", ">=", filter.From)
	where = addTime(where, parameters, "occurred_at", "to", "<=", filter.To)
	where = addEquals(where, parameters, [3]string{"service_name", "service", filter.Service}, [3]string{"metric_name", "metric", filter.Metric})
	parameters["limit"] = strconv.Itoa(filter.Limit)
	rows, err := queryRows[metricPointRow](ctx, store, `SELECT toString(id) AS id, metric_name, metric_unit, metric_type, service_name, occurred_at,
value, hist_count, hist_sum, attributes, exemplars FROM tracepilot_metric_points FINAL WHERE `+strings.Join(where, " AND ")+
		" ORDER BY occurred_at DESC, id DESC LIMIT {limit:UInt32}"+finalSettings+" FORMAT JSONEachRow", parameters)
	if err != nil {
		return nil, err
	}
	points := make([]analysis.MetricPoint, 0, len(rows))
	for _, row := range rows {
		point := analysis.MetricPoint{ID: row.ID, Name: row.Name, Unit: row.Unit, Type: row.Type, Service: row.Service,
			OccurredAt: row.OccurredAt, Value: row.Value, Count: row.Count, Sum: row.Sum, Attributes: make(map[string]any, len(row.Attributes))}
		for key, value := range row.Attributes {
			point.Attributes[key] = value
		}
		if err := json.Unmarshal([]byte(row.Exemplars), &point.Exemplars); err != nil {
			return nil, fmt.Errorf("decode metric exemplars: %w", err)
		}
		points = append(points, point)
	}
	return points, nil
}

type signalQualityRow struct {
	Spans        int64 `json:"spans"`
	OrphanSpans  int64 `json:"orphan_spans"`
	MetricPoints int64 `json:"metric_points"`
}

// SignalQuality counts spans, spans whose parent was never received, and
// metric points. Log counts come from LogQuality.
func (store *Store) SignalQuality(ctx context.Context, projectID string, filter analysis.Filter) (spans, orphans, points int64, err error) {
	where, parameters, err := store.scope(projectID, filter.EnvironmentID)
	if err != nil {
		return 0, 0, 0, err
	}
	spanWindow := addTime(append([]string{}, where...), parameters, "started_at", "from", ">=", filter.From)
	spanWindow = addTime(spanWindow, parameters, "started_at", "to", "<=", filter.To)
	spanWindow = addEquals(spanWindow, parameters, [3]string{"service_name", "service", filter.Service})
	parents := addTime(append([]string{}, where...), parameters, "started_at", "parent_from", ">=", parentStart(filter.From))
	parents = addTime(parents, parameters, "started_at", "to", "<=", filter.To)
	metricWindow := addTime(append([]string{}, where...), parameters, "occurred_at", "from", ">=", filter.From)
	metricWindow = addTime(metricWindow, parameters, "occurred_at", "to", "<=", filter.To)
	metricWindow = addEquals(metricWindow, parameters, [3]string{"service_name", "service", filter.Service})
	rows, err := queryRows[signalQualityRow](ctx, store, `SELECT
(SELECT count() FROM tracepilot_spans FINAL WHERE `+strings.Join(spanWindow, " AND ")+`) AS spans,
(SELECT count() FROM (SELECT trace_id, parent_span_id FROM tracepilot_spans FINAL WHERE `+strings.Join(spanWindow, " AND ")+` AND parent_span_id != '') AS child
  LEFT ANTI JOIN (SELECT trace_id, span_id FROM tracepilot_spans FINAL WHERE `+strings.Join(parents, " AND ")+`) AS parent
  ON child.trace_id = parent.trace_id AND child.parent_span_id = parent.span_id) AS orphan_spans,
(SELECT count() FROM tracepilot_metric_points FINAL WHERE `+strings.Join(metricWindow, " AND ")+`) AS metric_points`+finalSettings+" FORMAT JSONEachRow", parameters)
	if err != nil || len(rows) == 0 {
		return 0, 0, 0, err
	}
	return rows[0].Spans, rows[0].OrphanSpans, rows[0].MetricPoints, nil
}
