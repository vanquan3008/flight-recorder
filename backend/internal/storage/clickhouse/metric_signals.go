package clickhouse

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
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

// metricSignal is one series' mean in each window; NaN when a window has no usable sample.
type metricSignal struct {
	sample                        metricSample
	currentSamples, baselineCount int
	observed, baseline            float64
}

// DetectMetricEvidence reports series whose current mean deviates from their
// baseline: counters as rates over their own intervals (a new start time marks a
// reset), histograms as observation-weighted means, gauges and non-monotonic sums
// as plain means.
func (store *Store) DetectMetricEvidence(ctx context.Context, ingestion config.IngestionConfig, end time.Time) ([]analysis.DetectedEvidence, error) {
	where, parameters, err := store.scope(ingestion.ProjectID, ingestion.EnvironmentID)
	if err != nil {
		return nil, err
	}
	currentStart := end.Add(-currentWindow)
	baselineStart := currentStart.Add(-baselineWindow)
	where = addTime(where, parameters, "occurred_at", "from", ">=", baselineStart.Add(-counterLookback))
	where = addTime(where, parameters, "occurred_at", "end", "<", end)
	samples, err := queryRows[metricSample](ctx, store, `SELECT series_id, service_name, metric_name, metric_unit, metric_type, is_monotonic,
temporality, attributes, occurred_at, started_at, value, hist_count, hist_sum FROM tracepilot_metric_points FINAL WHERE `+
		strings.Join(where, " AND ")+" AND bitAnd(flags, 1) = 0 ORDER BY series_id, occurred_at, id"+finalSettings+" FORMAT JSONEachRow", parameters)
	if err != nil {
		return nil, fmt.Errorf("detect metric signals: %w", err)
	}
	result := make([]analysis.DetectedEvidence, 0)
	for _, signal := range metricSignals(samples, baselineStart, currentStart) {
		row := signal.sample
		direction := analysis.MetricDirection(row.Metric)
		scale, supported := analysis.MetricScale(row.Metric, row.Unit)
		if direction == "" || !supported || signal.currentSamples < 2 || signal.baselineCount < 2 || math.IsNaN(signal.observed) || math.IsNaN(signal.baseline) {
			continue
		}
		current, reference := signal.observed*scale, signal.baseline*scale
		if !analysis.IsDeviation(direction, current, reference) {
			continue
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
		evidence := detected(row.Service, "metric", entityType, entityID(entityType, entityName), row.Metric, direction, current, reference, currentStart, end, nil)
		evidence.Item.SeriesID = row.SeriesID
		evidence.Item.Summary += fmt.Sprintf(" (series %s)", row.SeriesID)
		result = append(result, evidence)
	}
	return result, nil
}

type windowTotals struct {
	samples, rows int
	sum, seconds  float64
	count         int64
	hasSum        bool
}

// metricSignals takes samples ordered by series and time and returns one signal per series.
func metricSignals(samples []metricSample, baselineStart, currentStart time.Time) []metricSignal {
	var signals []metricSignal
	for start := 0; start < len(samples); {
		end := start
		for end < len(samples) && samples[end].SeriesID == samples[start].SeriesID {
			end++
		}
		if signal, found := seriesSignal(samples[start:end], baselineStart, currentStart); found {
			signals = append(signals, signal)
		}
		start = end
	}
	return signals
}

func seriesSignal(series []metricSample, baselineStart, currentStart time.Time) (metricSignal, bool) {
	var current, baseline windowTotals
	included := false
	for index, point := range series {
		var previous *metricSample
		if index > 0 {
			previous = &series[index-1]
		}
		intervalStart, restart := counterInterval(point, previous, baselineStart)
		windowStart, totals := baselineStart, &baseline
		if !point.OccurredAt.Before(currentStart) {
			windowStart, totals = currentStart, &current
		}
		monotonicSum := point.Type == "sum" && point.IsMonotonic != nil && *point.IsMonotonic
		if point.OccurredAt.Before(baselineStart) || !(point.Type == "gauge" || (point.Type == "sum" && !monotonicSum) ||
			(intervalStart != nil && !intervalStart.Before(windowStart))) {
			continue
		}
		included = true
		elapsed := intervalStart != nil && point.OccurredAt.After(*intervalStart)
		switch {
		case monotonicSum:
			if delta := difference(point.Value, previousValue(previous), restart); delta != nil && *delta >= 0 && elapsed {
				totals.sum, totals.seconds, totals.samples, totals.hasSum = totals.sum+*delta, totals.seconds+point.OccurredAt.Sub(*intervalStart).Seconds(), totals.samples+1, true
			}
		case point.Type == "histogram":
			count, sum := countDifference(point.Count, previousCount(previous), restart), difference(point.Sum, previousSum(previous), restart)
			if count != nil && *count > 0 && sum != nil && elapsed {
				totals.count, totals.sum, totals.samples, totals.hasSum = totals.count+*count, totals.sum+*sum, totals.samples+1, true
			}
		case point.Value != nil:
			totals.sum, totals.rows, totals.samples = totals.sum+*point.Value, totals.rows+1, totals.samples+1
		}
	}
	if !included {
		return metricSignal{}, false
	}
	mean := func(totals windowTotals) float64 {
		switch {
		case series[0].Type == "histogram" && totals.hasSum && totals.count > 0:
			return totals.sum / float64(totals.count)
		case series[0].Type == "sum" && series[0].IsMonotonic != nil && *series[0].IsMonotonic && totals.hasSum && totals.seconds > 0:
			return totals.sum / totals.seconds
		case totals.rows > 0:
			return totals.sum / float64(totals.rows)
		}
		return math.NaN()
	}
	return metricSignal{sample: series[0], currentSamples: current.samples, baselineCount: baseline.samples,
		observed: mean(current), baseline: mean(baseline)}, true
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

func difference(value, previous *float64, restart bool) *float64 {
	if restart || value == nil {
		return value
	}
	if previous == nil {
		return nil
	}
	delta := *value - *previous
	return &delta
}

func countDifference(value, previous *int64, restart bool) *int64 {
	if restart || value == nil {
		return value
	}
	if previous == nil {
		return nil
	}
	delta := *value - *previous
	return &delta
}

func previousValue(sample *metricSample) *float64 {
	if sample == nil {
		return nil
	}
	return sample.Value
}

func previousCount(sample *metricSample) *int64 {
	if sample == nil {
		return nil
	}
	return sample.Count
}

func previousSum(sample *metricSample) *float64 {
	if sample == nil {
		return nil
	}
	return sample.Sum
}
