package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

func (store *AnalyticsStore) DetectMetricEvidence(ctx context.Context, ingestion config.IngestionConfig, end time.Time) ([]analysis.DetectedEvidence, error) {
	currentStart, baselineStart := end.Add(-currentWindow), end.Add(-currentWindow-baselineWindow)
	rows, err := store.queries.DetectMetricSignals(ctx, sqlcstore.DetectMetricSignalsParams{
		ProjectID: ingestion.ProjectID, EnvironmentID: ingestion.EnvironmentID,
		CurrentStart: currentStart, BaselineStart: baselineStart, WindowEnd: end,
	})
	if err != nil {
		return nil, fmt.Errorf("detect metric signals: %w", err)
	}
	result := make([]analysis.DetectedEvidence, 0)
	for _, row := range rows {
		metric, series := row.MetricName, row.SeriesID
		direction := analysis.MetricDirection(metric)
		scale, supported := analysis.MetricScale(metric, row.MetricUnit)
		// The query reports NaN for a window without a usable sample.
		if direction == "" || !supported || row.CurrentSamples < 2 || row.BaselineSamples < 2 || math.IsNaN(row.Observed) || math.IsNaN(row.Baseline) {
			continue
		}
		current, reference := row.Observed*scale, row.Baseline*scale
		if !analysis.IsDeviation(direction, current, reference) {
			continue
		}
		entityType, entityName := "service", row.ServiceName
		var dimensions map[string]any
		if err := json.Unmarshal(row.Attributes, &dimensions); err != nil {
			return nil, fmt.Errorf("decode metric dimensions: %w", err)
		}
		if strings.Contains(strings.ToLower(metric), "queue") || strings.Contains(strings.ToLower(metric), "backlog") {
			entityType, entityName = "queue", metric
			for _, key := range []string{"messaging.destination.name", "messaging.destination", "queue.name"} {
				if name, ok := dimensions[key].(string); ok && name != "" {
					entityName = name
					break
				}
			}
		}
		evidence := detected(row.ServiceName, "metric", entityType, pgvalue.EntityID(entityType, entityName), metric, direction, current, reference, currentStart, end, nil)
		evidence.Item.SeriesID = series
		evidence.Item.Summary += fmt.Sprintf(" (series %s)", series)
		result = append(result, evidence)
	}
	return result, nil
}
