package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
)

func (database *Database) DetectMetricEvidence(ctx context.Context, ingestion config.IngestionConfig, end time.Time) ([]analysis.DetectedEvidence, error) {
	currentStart, baselineStart := end.Add(-currentWindow), end.Add(-currentWindow-baselineWindow)
	rows, err := database.pool.Query(ctx, `
  WITH identified AS (
   SELECT *, md5(jsonb_build_array(service_name,service_version,metric_name,metric_unit,metric_type,
    temporality,is_monotonic,attributes,resource_attributes,scope_name,scope_version,scope_attributes)::text) series_id
   FROM metric_data_points WHERE project_id=$1 AND environment_id=$2
    AND occurred_at >= $4::timestamptz - interval '5 minutes' AND occurred_at < $5 AND (flags & 1)=0
  ), previous AS (
   SELECT *,lag(value) OVER series previous_value,lag(hist_count) OVER series previous_count,
    lag(hist_sum) OVER series previous_sum,lag(occurred_at) OVER series previous_time,
    lag(started_at) OVER series previous_start
   FROM identified WINDOW series AS (PARTITION BY series_id ORDER BY occurred_at,id)
  ), intervals AS (
   SELECT *,
    CASE WHEN temporality='delta' THEN started_at
     WHEN started_at > previous_time OR (previous_time IS NULL AND started_at >= $4) THEN started_at
     WHEN started_at IS NOT DISTINCT FROM previous_start THEN previous_time END interval_start,
    CASE WHEN temporality='delta' OR started_at > previous_time OR (previous_time IS NULL AND started_at >= $4)
     THEN value ELSE value-previous_value END delta_value,
    CASE WHEN temporality='delta' OR started_at > previous_time OR (previous_time IS NULL AND started_at >= $4)
     THEN hist_count ELSE hist_count-previous_count END delta_count,
    CASE WHEN temporality='delta' OR started_at > previous_time OR (previous_time IS NULL AND started_at >= $4)
     THEN hist_sum ELSE hist_sum-previous_sum END delta_sum
   FROM previous
  ), samples AS (
   SELECT *,
    CASE WHEN metric_type='sum' AND is_monotonic=true
      THEN CASE WHEN delta_value>=0 AND occurred_at>interval_start THEN delta_value END
     WHEN metric_type<>'histogram' THEN value END sample_value,
    CASE WHEN metric_type='sum' AND is_monotonic=true AND delta_value>=0 AND occurred_at>interval_start
      THEN extract(epoch FROM occurred_at-interval_start) END sample_seconds,
    CASE WHEN metric_type='histogram' AND delta_count>0 AND delta_sum IS NOT NULL AND occurred_at>interval_start THEN delta_count END sample_count,
    CASE WHEN metric_type='histogram' AND delta_count>0 AND delta_sum IS NOT NULL AND occurred_at>interval_start THEN delta_sum END sample_sum
   FROM intervals WHERE occurred_at >= $4
    AND (metric_type='gauge' OR (metric_type='sum' AND is_monotonic IS DISTINCT FROM true)
     OR interval_start >= CASE WHEN occurred_at >= $3 THEN $3 ELSE $4 END)
  )
  SELECT service_name,metric_name,metric_unit,metric_type,COALESCE(is_monotonic,false),series_id,attributes,
   count(*) FILTER (WHERE occurred_at >= $3 AND (sample_value IS NOT NULL OR sample_sum IS NOT NULL)),
   count(*) FILTER (WHERE occurred_at < $3 AND (sample_value IS NOT NULL OR sample_sum IS NOT NULL)),
   CASE WHEN metric_type='histogram' THEN sum(sample_sum) FILTER (WHERE occurred_at >= $3)/NULLIF(sum(sample_count) FILTER (WHERE occurred_at >= $3),0)
    WHEN metric_type='sum' AND is_monotonic=true THEN sum(sample_value) FILTER (WHERE occurred_at >= $3)/NULLIF(sum(sample_seconds) FILTER (WHERE occurred_at >= $3),0)
    ELSE avg(sample_value) FILTER (WHERE occurred_at >= $3) END,
   CASE WHEN metric_type='histogram' THEN sum(sample_sum) FILTER (WHERE occurred_at < $3)/NULLIF(sum(sample_count) FILTER (WHERE occurred_at < $3),0)
    WHEN metric_type='sum' AND is_monotonic=true THEN sum(sample_value) FILTER (WHERE occurred_at < $3)/NULLIF(sum(sample_seconds) FILTER (WHERE occurred_at < $3),0)
    ELSE avg(sample_value) FILTER (WHERE occurred_at < $3) END
  FROM samples GROUP BY service_name,metric_name,metric_unit,metric_type,is_monotonic,series_id,attributes
  ORDER BY service_name,metric_name,series_id`, ingestion.ProjectID, ingestion.EnvironmentID, currentStart, baselineStart, end)
	if err != nil {
		return nil, fmt.Errorf("detect metric signals: %w", err)
	}
	defer rows.Close()
	result := make([]analysis.DetectedEvidence, 0)
	for rows.Next() {
		var service, metric, unit, kind, series string
		var monotonic bool
		var attributes []byte
		var currentCount, baselineCount int64
		var observed, baseline *float64
		if err := rows.Scan(&service, &metric, &unit, &kind, &monotonic, &series, &attributes, &currentCount, &baselineCount, &observed, &baseline); err != nil {
			return nil, err
		}
		direction := analysis.MetricDirection(metric)
		scale, supported := analysis.MetricScale(metric, unit)
		if direction == "" || !supported || currentCount < 2 || baselineCount < 2 || observed == nil || baseline == nil {
			continue
		}
		current, reference := *observed*scale, *baseline*scale
		if !analysis.IsDeviation(direction, current, reference) {
			continue
		}
		entityType, entityName := "service", service
		var dimensions map[string]any
		if err := json.Unmarshal(attributes, &dimensions); err != nil {
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
		evidence := detected(service, "metric", entityType, entityID(entityType, entityName), metric, direction, current, reference, currentStart, end, nil)
		evidence.Item.SeriesID = series
		evidence.Item.Summary += fmt.Sprintf(" (series %s)", series)
		result = append(result, evidence)
	}
	return result, rows.Err()
}
