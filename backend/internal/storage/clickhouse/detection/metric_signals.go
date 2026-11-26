package detection

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"
)

// counterLookback reads points before the baseline so the first baseline
// counter sample has a previous value to take a delta from.
const counterLookback = 5 * time.Minute

type metricSample struct {
	SeriesID    string            `json:"series_id"`
	Service     string            `json:"service_name"`
	Metric      string            `json:"metric_name"`
	Unit        string            `json:"metric_unit"`
	Type        string            `json:"metric_type"`
	IsMonotonic *bool             `json:"is_monotonic"`
	Temporality string            `json:"temporality"`
	Attributes  map[string]string `json:"attributes"`
	OccurredAt  time.Time         `json:"occurred_at"`
	StartedAt   *time.Time        `json:"started_at"`
	Value       *float64          `json:"value"`
	Count       *int64            `json:"hist_count"`
	Sum         *float64          `json:"hist_sum"`
}

func (sample *metricSample) isMonotonicSum() bool {
	return sample.Type == "sum" && sample.IsMonotonic != nil && *sample.IsMonotonic
}

type metricSeries struct {
	first, previous   *metricSample
	current, baseline windowTotals
}

// DetectMetricEvidence reports series whose current mean deviates from their
// baseline: counters as rates over their own intervals (a new start time marks a
// reset), histograms as observation-weighted means, gauges and non-monotonic sums
// as plain means.
func (store *Detector) DetectMetricEvidence(ctx context.Context, ingestion config.IngestionConfig, end time.Time) ([]analysis.DetectedEvidence, error) {
	where, parameters, err := client.Scope(ingestion.ProjectID, ingestion.EnvironmentID)
	if err != nil {
		return nil, err
	}
	currentStart := end.Add(-currentWindow)
	baselineStart := currentStart.Add(-baselineWindow)
	where = client.AddTime(where, parameters, "occurred_at", "from", ">=", baselineStart.Add(-counterLookback))
	where = client.AddTime(where, parameters, "occurred_at", "end", "<", end)
	response, err := store.client.OpenResponse(ctx, fmt.Sprintf(metricEvidenceSQL, strings.Join(where, " AND "), client.FinalSettings), parameters, nil, false)
	if err != nil {
		return nil, fmt.Errorf("detect metric signals: %w", err)
	}
	defer response.Close()

	// Sorted rows let us keep only one series' current state in memory.
	result := make([]analysis.DetectedEvidence, 0)
	var series metricSeries
	for point, err := range client.DecodeJSONRows[metricSample](response) {
		if err != nil {
			return nil, fmt.Errorf("detect metric signals: %w", client.ResponseError(ctx, err, "decode ClickHouse rows"))
		}
		if series.first == nil || series.first.SeriesID != point.SeriesID {
			result = series.appendEvidence(result, currentStart, end)
			series = metricSeries{first: point}
		}
		series.add(point, baselineStart, currentStart)
	}
	return series.appendEvidence(result, currentStart, end), nil
}

func (series *metricSeries) appendEvidence(result []analysis.DetectedEvidence, currentStart, end time.Time) []analysis.DetectedEvidence {
	if series.current.samples < 2 || series.baseline.samples < 2 {
		return result
	}
	row := series.first
	direction := analysis.MetricDirection(row.Metric)
	scale, supported := analysis.MetricScale(row.Metric, row.Unit)
	if direction == "" || !supported {
		return result
	}
	current, reference := series.current.mean(row)*scale, series.baseline.mean(row)*scale
	if math.IsNaN(current) || math.IsNaN(reference) || !analysis.IsDeviation(direction, current, reference) {
		return result
	}
	entityType, entityName := "service", row.Service
	if name := strings.ToLower(row.Metric); strings.Contains(name, "queue") || strings.Contains(name, "backlog") {
		entityType, entityName = "queue", row.Metric
		for _, key := range []string{"messaging.destination.name", "messaging.destination", "queue.name"} {
			if value := row.Attributes[key]; value != "" {
				entityName = value
				break
			}
		}
	}
	evidence := detected(row.Service, "metric", entityType, analysis.EntityID(entityType, entityName), row.Metric, direction, current, reference, currentStart, end, nil)
	evidence.Item.SeriesID = row.SeriesID
	evidence.Item.Summary += fmt.Sprintf(" (series %s)", row.SeriesID)
	return append(result, evidence)
}

type windowTotals struct {
	samples, rows int
	sum, seconds  float64
	count         int64
	hasSum        bool
}

func (series *metricSeries) add(point *metricSample, baselineStart, currentStart time.Time) {
	previous := series.previous
	series.previous = point
	intervalStart, restart := counterInterval(*point, previous, baselineStart)
	windowStart, totals := baselineStart, &series.baseline
	if !point.OccurredAt.Before(currentStart) {
		windowStart, totals = currentStart, &series.current
	}
	monotonicSum := point.isMonotonicSum()
	if point.OccurredAt.Before(baselineStart) || !(point.Type == "gauge" || (point.Type == "sum" && !monotonicSum) ||
		(intervalStart != nil && !intervalStart.Before(windowStart))) {
		return
	}
	if previous == nil {
		previous = &metricSample{}
	}
	elapsed := intervalStart != nil && point.OccurredAt.After(*intervalStart)
	switch {
	case monotonicSum:
		if delta := metricDelta(point.Value, previous.Value, restart); delta != nil && *delta >= 0 && elapsed {
			totals.sum, totals.seconds, totals.samples, totals.hasSum = totals.sum+*delta, totals.seconds+point.OccurredAt.Sub(*intervalStart).Seconds(), totals.samples+1, true
		}
	case point.Type == "histogram":
		count, sum := metricDelta(point.Count, previous.Count, restart), metricDelta(point.Sum, previous.Sum, restart)
		if count != nil && *count > 0 && sum != nil && elapsed {
			totals.count, totals.sum, totals.samples, totals.hasSum = totals.count+*count, totals.sum+*sum, totals.samples+1, true
		}
	case point.Value != nil:
		totals.sum, totals.rows, totals.samples = totals.sum+*point.Value, totals.rows+1, totals.samples+1
	}
}

func (totals windowTotals) mean(sample *metricSample) float64 {
	switch {
	case sample.Type == "histogram" && totals.hasSum && totals.count > 0:
		return totals.sum / float64(totals.count)
	case sample.isMonotonicSum() && totals.hasSum && totals.seconds > 0:
		return totals.sum / totals.seconds
	case totals.rows > 0:
		return totals.sum / float64(totals.rows)
	}
	return math.NaN()
}

// counterInterval returns when the point's interval started and whether the
// point starts a new one (delta temporality, a reset, or the series' first
// point inside the window).
func counterInterval(point metricSample, previous *metricSample, baselineStart time.Time) (*time.Time, bool) {
	if point.Temporality == "delta" {
		return point.StartedAt, true
	}
	var previousTime, previousStart *time.Time
	if previous != nil {
		previousTime, previousStart = &previous.OccurredAt, previous.StartedAt
	}
	if point.StartedAt != nil && ((previousTime != nil && point.StartedAt.After(*previousTime)) ||
		(previousTime == nil && !point.StartedAt.Before(baselineStart))) {
		return point.StartedAt, true
	}
	if sameTime(point.StartedAt, previousStart) {
		return previousTime, false
	}
	return nil, false
}

func sameTime(first, second *time.Time) bool {
	if first == nil || second == nil {
		return first == nil && second == nil
	}
	return first.Equal(*second)
}

func metricDelta[T int64 | float64](value, previous *T, restart bool) *T {
	if restart || value == nil {
		return value
	}
	if previous == nil {
		return nil
	}
	delta := *value - *previous
	return &delta
}
