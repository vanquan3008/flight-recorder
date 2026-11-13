package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

const (
	currentWindow  = 5 * time.Minute
	baselineWindow = 30 * time.Minute
)

// spanKindConsumer is the OTLP span kind of a message consumer.
const spanKindConsumer = 5

func (store *AnalyticsStore) DetectServiceEvidence(ctx context.Context, ingestion config.IngestionConfig, end time.Time) ([]analysis.DetectedEvidence, error) {
	currentStart, baselineStart := end.Add(-currentWindow), end.Add(-currentWindow-baselineWindow)
	rows, err := store.queries.DetectServiceSignals(ctx, sqlcstore.DetectServiceSignalsParams{
		ProjectID: ingestion.ProjectID, EnvironmentID: ingestion.EnvironmentID,
		CurrentStart: currentStart, BaselineStart: baselineStart, WindowEnd: end,
	})
	if err != nil {
		return nil, fmt.Errorf("detect service signals: %w", err)
	}
	result := make([]analysis.DetectedEvidence, 0)
	for _, row := range rows {
		current := analysis.WindowStats{Calls: row.CurrentCalls, Errors: row.CurrentErrors, LatencyNanos: row.CurrentLatencyNanos}
		baseline := analysis.WindowStats{Calls: row.BaselineCalls, Errors: row.BaselineErrors, LatencyNanos: row.BaselineLatencyNanos}
		entity := pgvalue.EntityID("service", row.ServiceName)
		for _, signal := range analysis.EvaluateService(current, baseline, currentWindow, baselineWindow) {
			if row.SpanKind == spanKindConsumer {
				signal.Metric = "consumer." + signal.Metric
			}
			result = append(result, detected(row.ServiceName, "span", "service", entity, signal.Metric, signal.Direction, signal.Observed, signal.Baseline, currentStart, end, row.TraceIDs))
		}
	}
	return result, nil
}

func (store *AnalyticsStore) DetectDependencyEvidence(ctx context.Context, ingestion config.IngestionConfig, end time.Time) ([]analysis.DetectedEvidence, error) {
	currentStart, baselineStart := end.Add(-currentWindow), end.Add(-currentWindow-baselineWindow)
	rows, err := store.queries.DetectDependencySignals(ctx, sqlcstore.DetectDependencySignalsParams{
		ProjectID: ingestion.ProjectID, EnvironmentID: ingestion.EnvironmentID,
		CurrentStart: currentStart, BaselineStart: baselineStart, WindowEnd: end,
	})
	if err != nil {
		return nil, fmt.Errorf("detect dependency signals: %w", err)
	}
	result := make([]analysis.DetectedEvidence, 0)
	for _, row := range rows {
		current, baseline := row.CurrentLatencyNanos, row.BaselineLatencyNanos
		if row.CurrentCalls >= 2 && row.BaselineCalls >= 3 && current >= baseline*1.5 && current-baseline >= float64(50*time.Millisecond) {
			result = append(result, detected(row.ServiceName, "span", row.EntityType, pgvalue.EntityID(row.EntityType, row.EntityName), "latency", "increase", current, baseline, currentStart, end, row.TraceIDs))
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
	return analysis.DetectedEvidence{Service: service, Item: analysis.Evidence{SignalType: signal, EntityType: entityType, EntityID: entityID, MetricName: metric, Direction: direction, ObservedValue: observed, BaselineValue: baseline, ChangeRatio: ratio, WindowStartedAt: from, WindowEndedAt: to, TraceIDs: traces, Summary: summary}}
}
