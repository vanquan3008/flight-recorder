package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"
)

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

func (store *Analytics) ListMetricPoints(ctx context.Context, projectID string, filter analysis.Filter) ([]analysis.MetricPoint, error) {
	if filter.Limit < 1 {
		return []analysis.MetricPoint{}, nil
	}
	where, parameters, err := client.Scope(projectID, filter.EnvironmentID)
	if err != nil {
		return nil, err
	}
	where = client.AddTime(where, parameters, "occurred_at", "from", ">=", filter.From)
	where = client.AddTime(where, parameters, "occurred_at", "to", "<=", filter.To)
	where = client.AddEquals(where, parameters, [3]string{"service_name", "service", filter.Service}, [3]string{"metric_name", "metric", filter.Metric})
	parameters["limit"] = strconv.Itoa(filter.Limit)
	rows, err := client.QueryRows[metricPointRow](ctx, store.client, `SELECT
    toString(id) AS id,
    metric_name,
    metric_unit,
    metric_type,
    service_name,
    occurred_at,
    value,
    hist_count,
    hist_sum,
    attributes,
    exemplars
FROM tracepilot_metric_points FINAL
WHERE `+strings.Join(where, " AND ")+`
ORDER BY occurred_at DESC, id DESC
LIMIT {limit:UInt32}`+client.FinalSettings+`
FORMAT JSONEachRow`, parameters)
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

// SignalQuality counts spans, spans with no retained parent in the lookback, and
// metric points. Log counts come from LogQuality.
func (store *Analytics) SignalQuality(ctx context.Context, projectID string, filter analysis.Filter) (spans, orphans, points int64, err error) {
	where, parameters, err := client.Scope(projectID, filter.EnvironmentID)
	if err != nil {
		return 0, 0, 0, err
	}
	filter.From = client.MaxTime(filter.From, filter.Since)
	spanWindow := client.AddTime(append([]string{}, where...), parameters, "started_at", "from", ">=", filter.From)
	spanWindow = client.AddTime(spanWindow, parameters, "started_at", "to", "<=", filter.To)
	spanWindow = client.AddEquals(spanWindow, parameters, [3]string{"service_name", "service", filter.Service})
	parents := client.AddTime(append([]string{}, where...), parameters, "started_at", "parent_from", ">=", client.MaxTime(parentStart(filter.From), filter.Since))
	parents = client.AddTime(parents, parameters, "started_at", "to", "<=", filter.To)
	metricWindow := client.AddTime(append([]string{}, where...), parameters, "occurred_at", "from", ">=", filter.From)
	metricWindow = client.AddTime(metricWindow, parameters, "occurred_at", "to", "<=", filter.To)
	metricWindow = client.AddEquals(metricWindow, parameters, [3]string{"service_name", "service", filter.Service})
	rows, err := client.QueryRows[signalQualityRow](ctx, store.client, `SELECT
    (
        SELECT count()
        FROM tracepilot_spans FINAL
        WHERE `+strings.Join(spanWindow, " AND ")+`
    ) AS spans,
    (
        SELECT count()
        FROM (
            SELECT trace_id, parent_span_id
            FROM tracepilot_spans FINAL
            WHERE `+strings.Join(spanWindow, " AND ")+`
              AND parent_span_id != ''
        ) AS child
        LEFT ANTI JOIN (
            SELECT trace_id, span_id
            FROM tracepilot_spans FINAL
            WHERE `+strings.Join(parents, " AND ")+`
        ) AS parent
            ON child.trace_id = parent.trace_id
           AND child.parent_span_id = parent.span_id
    ) AS orphan_spans,
    (
        SELECT count()
        FROM tracepilot_metric_points FINAL
        WHERE `+strings.Join(metricWindow, " AND ")+`
    ) AS metric_points`+client.FinalSettings+`
FORMAT JSONEachRow`, parameters)
	if err != nil || len(rows) == 0 {
		return 0, 0, 0, err
	}
	return rows[0].Spans, rows[0].OrphanSpans, rows[0].MetricPoints, nil
}
