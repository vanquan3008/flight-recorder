package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
)

const (
	currentWindow  = 5 * time.Minute
	baselineWindow = 30 * time.Minute
)

func (database *Database) DetectServiceEvidence(ctx context.Context, ingestion config.IngestionConfig, end time.Time) ([]analysis.DetectedEvidence, error) {
	currentStart, baselineStart := end.Add(-currentWindow), end.Add(-currentWindow-baselineWindow)
	rows, err := database.pool.Query(ctx, `
		SELECT service_name,span_kind,
		 count(*) FILTER (WHERE started_at >= $3), count(*) FILTER (WHERE started_at < $3),
		 count(*) FILTER (WHERE started_at >= $3 AND status_code='ERROR'), count(*) FILTER (WHERE started_at < $3 AND status_code='ERROR'),
		 COALESCE(avg(extract(epoch FROM ended_at-started_at)*1000000000) FILTER (WHERE started_at >= $3),0),
		 COALESCE(avg(extract(epoch FROM ended_at-started_at)*1000000000) FILTER (WHERE started_at < $3),0),
		 (array_agg(DISTINCT trace_id ORDER BY trace_id) FILTER (WHERE started_at >= $3))[1:5]
		FROM spans WHERE project_id=$1 AND environment_id=$2 AND span_kind IN (2,5) AND started_at >= $4 AND started_at < $5
		GROUP BY service_name,span_kind`, ingestion.ProjectID, ingestion.EnvironmentID, currentStart, baselineStart, end)
	if err != nil {
		return nil, fmt.Errorf("detect service signals: %w", err)
	}
	defer rows.Close()
	result := make([]analysis.DetectedEvidence, 0)
	for rows.Next() {
		var service string
		var kind int32
		var current, baseline analysis.WindowStats
		var traces []string
		if err := rows.Scan(&service, &kind, &current.Calls, &baseline.Calls, &current.Errors, &baseline.Errors, &current.LatencyNanos, &baseline.LatencyNanos, &traces); err != nil {
			return nil, err
		}
		entity := entityID("service", service)
		for _, signal := range analysis.EvaluateService(current, baseline, currentWindow, baselineWindow) {
			if kind == 5 {
				signal.Metric = "consumer." + signal.Metric
			}
			result = append(result, detected(service, "span", "service", entity, signal.Metric, signal.Direction, signal.Observed, signal.Baseline, currentStart, end, traces))
		}
	}
	return result, rows.Err()
}

func (database *Database) DetectDependencyEvidence(ctx context.Context, ingestion config.IngestionConfig, end time.Time) ([]analysis.DetectedEvidence, error) {
	currentStart, baselineStart := end.Add(-currentWindow), end.Add(-currentWindow-baselineWindow)
	rows, err := database.pool.Query(ctx, `
		WITH candidates AS (
		 SELECT service_name,trace_id,status_code,started_at,ended_at,
		  `+dependencyTypeSQL+` entity_type, `+dependencyNameSQL+` entity_name
		 FROM spans WHERE project_id=$1 AND environment_id=$2 AND started_at >= $4 AND started_at < $5
		  AND `+dependencyPredicateSQL+`
		)
		SELECT service_name,entity_type,entity_name,
		 count(*) FILTER (WHERE started_at >= $3),count(*) FILTER (WHERE started_at < $3),
		 COALESCE(avg(extract(epoch FROM ended_at-started_at)*1000000000) FILTER (WHERE started_at >= $3),0),
		 COALESCE(avg(extract(epoch FROM ended_at-started_at)*1000000000) FILTER (WHERE started_at < $3),0),
		 (array_agg(DISTINCT trace_id ORDER BY trace_id) FILTER (WHERE started_at >= $3))[1:5]
		FROM candidates WHERE entity_name IS NOT NULL AND entity_name <> ''
		GROUP BY service_name,entity_type,entity_name`, ingestion.ProjectID, ingestion.EnvironmentID, currentStart, baselineStart, end)
	if err != nil {
		return nil, fmt.Errorf("detect dependency signals: %w", err)
	}
	defer rows.Close()
	result := make([]analysis.DetectedEvidence, 0)
	for rows.Next() {
		var service, entityType, entityName string
		var currentCalls, baselineCalls int64
		var currentLatency, baselineLatency float64
		var traces []string
		if err := rows.Scan(&service, &entityType, &entityName, &currentCalls, &baselineCalls, &currentLatency, &baselineLatency, &traces); err != nil {
			return nil, err
		}
		if currentCalls >= 2 && baselineCalls >= 3 && currentLatency >= baselineLatency*1.5 && currentLatency-baselineLatency >= float64(50*time.Millisecond) {
			result = append(result, detected(service, "span", entityType, entityID(entityType, entityName), "latency", "increase", currentLatency, baselineLatency, currentStart, end, traces))
		}
	}
	return result, rows.Err()
}

func detected(service, signal, entityType, entityID, metric, direction string, observed, baseline float64, from, to time.Time, traces []string) analysis.DetectedEvidence {
	ratio := 1.0
	if baseline != 0 {
		ratio = observed / baseline
	}
	summary := fmt.Sprintf("%s %s on %s: %.4g vs baseline %.4g", metric, direction, entityID, observed, baseline)
	return analysis.DetectedEvidence{Service: service, Item: analysis.Evidence{SignalType: signal, EntityType: entityType, EntityID: entityID, MetricName: metric, Direction: direction, ObservedValue: observed, BaselineValue: baseline, ChangeRatio: ratio, WindowStartedAt: from, WindowEndedAt: to, TraceIDs: traces, Summary: summary}}
}
