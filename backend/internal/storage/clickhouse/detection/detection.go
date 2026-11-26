package detection

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"
)

const (
	currentWindow  = 5 * time.Minute
	baselineWindow = 30 * time.Minute
)

// spanKindConsumer is the OTLP span kind of a message consumer.
const spanKindConsumer = 5

type windowSignalRow struct {
	ServiceName          string   `json:"service_name"`
	SpanKind             int32    `json:"span_kind"`
	EntityType           string   `json:"entity_type"`
	EntityName           string   `json:"entity_name"`
	CurrentCalls         int64    `json:"current_calls"`
	BaselineCalls        int64    `json:"baseline_calls"`
	CurrentErrors        int64    `json:"current_errors"`
	BaselineErrors       int64    `json:"baseline_errors"`
	CurrentLatencyNanos  float64  `json:"current_latency_nanos"`
	BaselineLatencyNanos float64  `json:"baseline_latency_nanos"`
	TraceIDs             []string `json:"trace_ids"`
}

const windowSignalColumns = `
    countIf(started_at >= {current:DateTime64(9, 'UTC')}) AS current_calls,
    countIf(started_at < {current:DateTime64(9, 'UTC')}) AS baseline_calls,
    countIf(started_at >= {current:DateTime64(9, 'UTC')} AND status_code = 'ERROR') AS current_errors,
    countIf(started_at < {current:DateTime64(9, 'UTC')} AND status_code = 'ERROR') AS baseline_errors,
    if(current_calls = 0, 0,
       sumIf(duration_ns, started_at >= {current:DateTime64(9, 'UTC')}) / current_calls) AS current_latency_nanos,
    if(baseline_calls = 0, 0,
       sumIf(duration_ns, started_at < {current:DateTime64(9, 'UTC')}) / baseline_calls) AS baseline_latency_nanos,
    arraySlice(arraySort(groupUniqArrayIf(trace_id, started_at >= {current:DateTime64(9, 'UTC')})), 1, 5) AS trace_ids`

// windowSignals compares the five minutes before end with the thirty before them.
func (store *Detector) windowSignals(ctx context.Context, ingestion config.IngestionConfig, end time.Time, columns, condition, groups string) ([]windowSignalRow, time.Time, error) {
	where, parameters, err := client.Scope(ingestion.ProjectID, ingestion.EnvironmentID)
	if err != nil {
		return nil, time.Time{}, err
	}
	currentStart := end.Add(-currentWindow)
	parameters["current"] = client.Time(currentStart)
	where = client.AddTime(where, parameters, "started_at", "baseline", ">=", currentStart.Add(-baselineWindow))
	where = client.AddTime(where, parameters, "started_at", "end", "<", end)
	rows, err := client.QueryRows[windowSignalRow](ctx, store.client, `SELECT
    `+columns+`,`+windowSignalColumns+`
FROM tracepilot_spans FINAL
WHERE `+strings.Join(where, " AND ")+`
  AND `+condition+`
GROUP BY `+groups+client.FinalSettings+`
FORMAT JSONEachRow`, parameters)
	return rows, currentStart, err
}

// DetectServiceEvidence evaluates inbound (SERVER) and consumer spans per service.
func (store *Detector) DetectServiceEvidence(ctx context.Context, ingestion config.IngestionConfig, end time.Time) ([]analysis.DetectedEvidence, error) {
	rows, currentStart, err := store.windowSignals(ctx, ingestion, end, "service_name, span_kind", "span_kind IN (2, 5)", "service_name, span_kind")
	if err != nil {
		return nil, fmt.Errorf("detect service signals: %w", err)
	}
	result := make([]analysis.DetectedEvidence, 0)
	for _, row := range rows {
		current := analysis.WindowStats{Calls: row.CurrentCalls, Errors: row.CurrentErrors, LatencyNanos: row.CurrentLatencyNanos}
		baseline := analysis.WindowStats{Calls: row.BaselineCalls, Errors: row.BaselineErrors, LatencyNanos: row.BaselineLatencyNanos}
		entity := analysis.EntityID("service", row.ServiceName)
		for _, signal := range analysis.EvaluateService(current, baseline, currentWindow, baselineWindow) {
			if row.SpanKind == spanKindConsumer {
				signal.Metric = "consumer." + signal.Metric
			}
			result = append(result, detected(row.ServiceName, "span", "service", entity, signal.Metric, signal.Direction, signal.Observed, signal.Baseline, currentStart, end, row.TraceIDs))
		}
	}
	return result, nil
}

// DetectDependencyEvidence reports a dependency whose latency rose by half and by at least 50 ms.
func (store *Detector) DetectDependencyEvidence(ctx context.Context, ingestion config.IngestionConfig, end time.Time) ([]analysis.DetectedEvidence, error) {
	rows, currentStart, err := store.windowSignals(ctx, ingestion, end, "service_name, dependency_type AS entity_type, dependency_name AS entity_name",
		"dependency_type != '' AND dependency_name != ''", "service_name, entity_type, entity_name")
	if err != nil {
		return nil, fmt.Errorf("detect dependency signals: %w", err)
	}
	result := make([]analysis.DetectedEvidence, 0)
	for _, row := range rows {
		current, baseline := row.CurrentLatencyNanos, row.BaselineLatencyNanos
		if row.CurrentCalls >= 2 && row.BaselineCalls >= 3 && current >= baseline*1.5 && current-baseline >= float64(50*time.Millisecond) {
			result = append(result, detected(row.ServiceName, "span", row.EntityType, analysis.EntityID(row.EntityType, row.EntityName), "latency", "increase", current, baseline, currentStart, end, row.TraceIDs))
		}
	}
	return result, nil
}

func detected(service, signal, entityType, entityID, metric, direction string, observed, baseline float64, from, to time.Time, traces []string) analysis.DetectedEvidence {
	ratio := 1.0
	if baseline != 0 {
		ratio = observed / baseline
	}
	summary := fmt.Sprintf("%s %s on %s: %.4g vs baseline %.4g", metric, direction, entityID, observed, baseline)
	return analysis.DetectedEvidence{Service: service, Item: analysis.Evidence{SignalType: signal, EntityType: entityType, EntityID: entityID,
		MetricName: metric, Direction: direction, ObservedValue: observed, BaselineValue: baseline, ChangeRatio: ratio,
		WindowStartedAt: from, WindowEndedAt: to, TraceIDs: traces, Summary: summary}}
}
