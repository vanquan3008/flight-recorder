-- Edges between services (parent span -> child span in another service) and
-- from services to the databases, queues and external targets they call.
-- name: ListTopologyEdges :many
WITH window_spans AS (
    SELECT * FROM spans
    WHERE spans.project_id = @project_id
      AND spans.environment_id = @environment_id
      AND spans.started_at >= @from_time::timestamptz
      AND spans.started_at < @to_time::timestamptz
), raw_edges AS (
    SELECT parent.service_name AS source_name, 'service' AS target_type, child.service_name AS target_name,
           'service' AS edge_kind, child.trace_id, child.status_code,
           extract(epoch FROM child.ended_at - child.started_at) * 1000000000 AS duration_ns
    FROM window_spans AS child
    JOIN spans AS parent
      ON parent.project_id = child.project_id
     AND parent.environment_id = child.environment_id
     AND parent.trace_id = child.trace_id
     AND parent.span_id = child.parent_span_id
    WHERE parent.service_name <> child.service_name
    UNION ALL
    SELECT window_spans.service_name, span_dependency_type(window_spans.span_attributes),
           span_dependency_name(window_spans.span_attributes), span_dependency_type(window_spans.span_attributes),
           window_spans.trace_id, window_spans.status_code,
           extract(epoch FROM window_spans.ended_at - window_spans.started_at) * 1000000000
    FROM window_spans
    WHERE span_is_dependency(window_spans.span_attributes, window_spans.span_kind)
)
SELECT raw_edges.source_name::text AS source_name, raw_edges.target_type::text AS target_type,
       raw_edges.target_name::text AS target_name, raw_edges.edge_kind::text AS edge_kind,
       count(*) AS call_count,
       count(*) FILTER (WHERE raw_edges.status_code = 'ERROR') AS error_count,
       COALESCE(avg(raw_edges.duration_ns), 0)::bigint AS average_latency_nanos,
       COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY raw_edges.duration_ns), 0)::bigint AS p95_latency_nanos,
       (array_agg(DISTINCT raw_edges.trace_id ORDER BY raw_edges.trace_id))[1:5]::text[] AS trace_ids
FROM raw_edges
WHERE raw_edges.target_name IS NOT NULL AND raw_edges.target_name <> ''
  AND (sqlc.narg(service)::text IS NULL
       OR raw_edges.source_name = sqlc.narg(service)
       OR (raw_edges.target_type = 'service' AND raw_edges.target_name = sqlc.narg(service)))
GROUP BY raw_edges.source_name, raw_edges.target_type, raw_edges.target_name, raw_edges.edge_kind
ORDER BY raw_edges.source_name, raw_edges.target_type, raw_edges.target_name;

-- name: ListMetricPoints :many
SELECT id, metric_name, metric_unit, metric_type, service_name, occurred_at, value, hist_count, hist_sum, attributes, exemplars
FROM metric_data_points
WHERE project_id = @project_id
  AND environment_id = @environment_id
  AND occurred_at >= @from_time::timestamptz
  AND occurred_at <= @to_time::timestamptz
  AND (sqlc.narg(service)::text IS NULL OR service_name = sqlc.narg(service))
  AND (sqlc.narg(metric)::text IS NULL OR metric_name = sqlc.narg(metric))
ORDER BY occurred_at DESC, id DESC
LIMIT sqlc.arg(row_limit)::integer;

-- name: TelemetryQuality :one
SELECT
    (SELECT count(*) FROM spans AS s
     WHERE s.project_id = @project_id AND s.environment_id = @environment_id
       AND s.started_at BETWEEN @from_time::timestamptz AND @to_time::timestamptz
       AND (sqlc.narg(service)::text IS NULL OR s.service_name = sqlc.narg(service))) AS spans,
    (SELECT count(*) FROM spans AS s
     WHERE s.project_id = @project_id AND s.environment_id = @environment_id
       AND s.started_at BETWEEN @from_time::timestamptz AND @to_time::timestamptz
       AND s.parent_span_id IS NOT NULL
       AND (sqlc.narg(service)::text IS NULL OR s.service_name = sqlc.narg(service))
       AND NOT EXISTS (
           SELECT 1 FROM spans AS p
           WHERE p.project_id = s.project_id AND p.environment_id = s.environment_id
             AND p.trace_id = s.trace_id AND p.span_id = s.parent_span_id)) AS orphan_spans,
    (SELECT count(*) FROM logs AS l
     WHERE l.project_id = @project_id AND l.environment_id = @environment_id
       AND l.occurred_at BETWEEN @from_time::timestamptz AND @to_time::timestamptz
       AND (sqlc.narg(service)::text IS NULL OR l.service_name = sqlc.narg(service))) AS logs,
    (SELECT count(*) FROM logs AS l
     WHERE l.project_id = @project_id AND l.environment_id = @environment_id
       AND l.occurred_at BETWEEN @from_time::timestamptz AND @to_time::timestamptz
       AND l.trace_id IS NULL
       AND (sqlc.narg(service)::text IS NULL OR l.service_name = sqlc.narg(service))) AS uncorrelated_logs,
    (SELECT count(*) FROM metric_data_points AS m
     WHERE m.project_id = @project_id AND m.environment_id = @environment_id
       AND m.occurred_at BETWEEN @from_time::timestamptz AND @to_time::timestamptz
       AND (sqlc.narg(service)::text IS NULL OR m.service_name = sqlc.narg(service))) AS metric_points;

-- Inbound (SERVER, kind 2) and consumer (kind 5) spans per service, split
-- into the current window and the baseline before it.
-- name: DetectServiceSignals :many
SELECT service_name, span_kind,
       count(*) FILTER (WHERE started_at >= @current_start::timestamptz) AS current_calls,
       count(*) FILTER (WHERE started_at < @current_start::timestamptz) AS baseline_calls,
       count(*) FILTER (WHERE started_at >= @current_start::timestamptz AND status_code = 'ERROR') AS current_errors,
       count(*) FILTER (WHERE started_at < @current_start::timestamptz AND status_code = 'ERROR') AS baseline_errors,
       COALESCE(avg(extract(epoch FROM ended_at - started_at) * 1000000000)
           FILTER (WHERE started_at >= @current_start::timestamptz), 0)::double precision AS current_latency_nanos,
       COALESCE(avg(extract(epoch FROM ended_at - started_at) * 1000000000)
           FILTER (WHERE started_at < @current_start::timestamptz), 0)::double precision AS baseline_latency_nanos,
       COALESCE((array_agg(DISTINCT trace_id ORDER BY trace_id)
           FILTER (WHERE started_at >= @current_start::timestamptz))[1:5], '{}')::text[] AS trace_ids
FROM spans
WHERE project_id = @project_id
  AND environment_id = @environment_id
  AND span_kind IN (2, 5)
  AND started_at >= @baseline_start::timestamptz
  AND started_at < @window_end::timestamptz
GROUP BY service_name, span_kind;

-- name: DetectDependencySignals :many
WITH candidates AS (
    SELECT spans.service_name, spans.trace_id, spans.started_at, spans.ended_at,
           span_dependency_type(spans.span_attributes) AS entity_type,
           span_dependency_name(spans.span_attributes) AS entity_name
    FROM spans
    WHERE spans.project_id = @project_id
      AND spans.environment_id = @environment_id
      AND spans.started_at >= @baseline_start::timestamptz
      AND spans.started_at < @window_end::timestamptz
      AND span_is_dependency(spans.span_attributes, spans.span_kind)
)
SELECT candidates.service_name, candidates.entity_type::text AS entity_type, candidates.entity_name::text AS entity_name,
       count(*) FILTER (WHERE candidates.started_at >= @current_start::timestamptz) AS current_calls,
       count(*) FILTER (WHERE candidates.started_at < @current_start::timestamptz) AS baseline_calls,
       COALESCE(avg(extract(epoch FROM candidates.ended_at - candidates.started_at) * 1000000000)
           FILTER (WHERE candidates.started_at >= @current_start::timestamptz), 0)::double precision AS current_latency_nanos,
       COALESCE(avg(extract(epoch FROM candidates.ended_at - candidates.started_at) * 1000000000)
           FILTER (WHERE candidates.started_at < @current_start::timestamptz), 0)::double precision AS baseline_latency_nanos,
       COALESCE((array_agg(DISTINCT candidates.trace_id ORDER BY candidates.trace_id)
           FILTER (WHERE candidates.started_at >= @current_start::timestamptz))[1:5], '{}')::text[] AS trace_ids
FROM candidates
WHERE candidates.entity_name IS NOT NULL AND candidates.entity_name <> ''
GROUP BY candidates.service_name, candidates.entity_type, candidates.entity_name;

-- Per metric series: counters become rates over their own intervals (handling
-- resets via start time), histograms become sum/count means, gauges and
-- non-monotonic sums are averaged. Observed and baseline are NaN when the
-- window has no usable sample.
-- name: DetectMetricSignals :many
WITH identified AS (
    SELECT metric_data_points.*,
        md5(jsonb_build_array(service_name, service_version, metric_name, metric_unit, metric_type,
            temporality, is_monotonic, attributes, resource_attributes, scope_name, scope_version,
            scope_attributes)::text) AS series_id
    FROM metric_data_points
    WHERE project_id = @project_id
      AND environment_id = @environment_id
      AND occurred_at >= sqlc.arg(baseline_start)::timestamptz - interval '5 minutes'
      AND occurred_at < @window_end::timestamptz
      AND (flags & 1) = 0
), previous AS (
    SELECT identified.*,
        lag(value) OVER series AS previous_value,
        lag(hist_count) OVER series AS previous_count,
        lag(hist_sum) OVER series AS previous_sum,
        lag(occurred_at) OVER series AS previous_time,
        lag(started_at) OVER series AS previous_start
    FROM identified
    WINDOW series AS (PARTITION BY series_id ORDER BY occurred_at, id)
), intervals AS (
    SELECT previous.*,
        CASE WHEN temporality = 'delta' THEN started_at
             WHEN started_at > previous_time OR (previous_time IS NULL AND started_at >= @baseline_start::timestamptz) THEN started_at
             WHEN started_at IS NOT DISTINCT FROM previous_start THEN previous_time
        END AS interval_start,
        CASE WHEN temporality = 'delta' OR started_at > previous_time OR (previous_time IS NULL AND started_at >= @baseline_start::timestamptz)
             THEN value ELSE value - previous_value END AS delta_value,
        CASE WHEN temporality = 'delta' OR started_at > previous_time OR (previous_time IS NULL AND started_at >= @baseline_start::timestamptz)
             THEN hist_count ELSE hist_count - previous_count END AS delta_count,
        CASE WHEN temporality = 'delta' OR started_at > previous_time OR (previous_time IS NULL AND started_at >= @baseline_start::timestamptz)
             THEN hist_sum ELSE hist_sum - previous_sum END AS delta_sum
    FROM previous
), samples AS (
    SELECT intervals.*,
        CASE WHEN metric_type = 'sum' AND is_monotonic = true
                 THEN CASE WHEN delta_value >= 0 AND occurred_at > interval_start THEN delta_value END
             WHEN metric_type <> 'histogram' THEN value
        END AS sample_value,
        CASE WHEN metric_type = 'sum' AND is_monotonic = true AND delta_value >= 0 AND occurred_at > interval_start
             THEN extract(epoch FROM occurred_at - interval_start) END AS sample_seconds,
        CASE WHEN metric_type = 'histogram' AND delta_count > 0 AND delta_sum IS NOT NULL AND occurred_at > interval_start
             THEN delta_count END AS sample_count,
        CASE WHEN metric_type = 'histogram' AND delta_count > 0 AND delta_sum IS NOT NULL AND occurred_at > interval_start
             THEN delta_sum END AS sample_sum
    FROM intervals
    WHERE occurred_at >= @baseline_start::timestamptz
      AND (metric_type = 'gauge'
           OR (metric_type = 'sum' AND is_monotonic IS DISTINCT FROM true)
           OR interval_start >= CASE WHEN occurred_at >= @current_start::timestamptz
                                     THEN @current_start::timestamptz ELSE @baseline_start::timestamptz END)
)
SELECT samples.service_name, samples.metric_name, samples.metric_unit, samples.metric_type,
    COALESCE(samples.is_monotonic, false)::boolean AS is_monotonic, samples.series_id::text AS series_id, samples.attributes,
    count(*) FILTER (WHERE samples.occurred_at >= @current_start::timestamptz
                       AND (samples.sample_value IS NOT NULL OR samples.sample_sum IS NOT NULL)) AS current_samples,
    count(*) FILTER (WHERE samples.occurred_at < @current_start::timestamptz
                       AND (samples.sample_value IS NOT NULL OR samples.sample_sum IS NOT NULL)) AS baseline_samples,
    COALESCE(CASE
        WHEN samples.metric_type = 'histogram' THEN
            sum(samples.sample_sum) FILTER (WHERE samples.occurred_at >= @current_start::timestamptz)
            / NULLIF(sum(samples.sample_count) FILTER (WHERE samples.occurred_at >= @current_start::timestamptz), 0)
        WHEN samples.metric_type = 'sum' AND samples.is_monotonic = true THEN
            sum(samples.sample_value) FILTER (WHERE samples.occurred_at >= @current_start::timestamptz)
            / NULLIF(sum(samples.sample_seconds) FILTER (WHERE samples.occurred_at >= @current_start::timestamptz), 0)
        ELSE avg(samples.sample_value) FILTER (WHERE samples.occurred_at >= @current_start::timestamptz)
    END, 'NaN')::double precision AS observed,
    COALESCE(CASE
        WHEN samples.metric_type = 'histogram' THEN
            sum(samples.sample_sum) FILTER (WHERE samples.occurred_at < @current_start::timestamptz)
            / NULLIF(sum(samples.sample_count) FILTER (WHERE samples.occurred_at < @current_start::timestamptz), 0)
        WHEN samples.metric_type = 'sum' AND samples.is_monotonic = true THEN
            sum(samples.sample_value) FILTER (WHERE samples.occurred_at < @current_start::timestamptz)
            / NULLIF(sum(samples.sample_seconds) FILTER (WHERE samples.occurred_at < @current_start::timestamptz), 0)
        ELSE avg(samples.sample_value) FILTER (WHERE samples.occurred_at < @current_start::timestamptz)
    END, 'NaN')::double precision AS baseline
FROM samples
GROUP BY samples.service_name, samples.metric_name, samples.metric_unit, samples.metric_type,
         samples.is_monotonic, samples.series_id, samples.attributes
ORDER BY samples.service_name, samples.metric_name, samples.series_id;

-- The detector's summary event; its evidence rows are stored separately.
-- name: InsertObservabilityEvent :exec
INSERT INTO events (id, project_id, environment_id, service_name, event_type, occurred_at, severity, status,
                    operation, error_type, error_message, attributes)
VALUES (@id, @project_id, @environment_id, @service_name, 'CUSTOM', @occurred_at, 'ERROR', 'ERROR',
        @operation, @error_type, @error_message, @attributes)
ON CONFLICT (id, occurred_at) DO NOTHING;

-- name: GetOccurrenceIncidentID :one
SELECT incident_id FROM incident_occurrences
WHERE project_id = @project_id AND event_id = @event_id;

-- name: InsertDetectedEvidence :exec
INSERT INTO incident_evidence (
    id, project_id, environment_id, incident_id, signal_type, entity_type, entity_id, metric_name,
    direction, observed_value, baseline_value, change_ratio, window_started_at, window_ended_at,
    trace_ids, summary, series_id
) VALUES (
    @id, @project_id, @environment_id, @incident_id, @signal_type, @entity_type, @entity_id, @metric_name,
    @direction, @observed_value, @baseline_value, @change_ratio, @window_started_at, @window_ended_at,
    @trace_ids, @summary, @series_id
)
ON CONFLICT DO NOTHING;

-- name: GetIncidentPrimaryService :one
SELECT primary_service FROM incidents
WHERE project_id = @project_id AND environment_id = @environment_id AND id = @id;

-- The latest window of each evidence series, plus direct error events from
-- the five minutes before it.
-- name: ListLatestIncidentEvidence :many
WITH selected_window AS (
    SELECT max(window_ended_at) AS ended_at FROM incident_evidence
    WHERE incident_evidence.project_id = @project_id
      AND incident_evidence.environment_id = @environment_id
      AND incident_evidence.incident_id = @incident_id
)
SELECT latest.id, latest.signal_type, latest.entity_type, latest.entity_id, latest.metric_name, latest.direction,
       latest.observed_value, latest.baseline_value, latest.change_ratio, latest.window_started_at,
       latest.window_ended_at, latest.trace_ids, latest.summary, latest.series_id
FROM (
    SELECT DISTINCT ON (evidence.signal_type, evidence.entity_id, evidence.metric_name, evidence.series_id)
        evidence.*
    FROM incident_evidence AS evidence
    CROSS JOIN selected_window
    WHERE evidence.project_id = @project_id
      AND evidence.environment_id = @environment_id
      AND evidence.incident_id = @incident_id
      AND (evidence.window_ended_at = selected_window.ended_at
           OR (evidence.signal_type = 'event'
               AND evidence.window_ended_at >= selected_window.ended_at - interval '5 minutes'))
    ORDER BY evidence.signal_type, evidence.entity_id, evidence.metric_name, evidence.series_id,
             evidence.window_ended_at DESC, evidence.id DESC
) AS latest
ORDER BY latest.window_ended_at DESC, latest.id DESC
LIMIT 100;

-- name: ListServices :many
SELECT
    service_name AS name,
    count(DISTINCT trace_id) AS trace_count,
    count(*) FILTER (WHERE span_kind = 2) AS request_count,
    count(*) FILTER (WHERE status_code = 'ERROR') AS error_count,
    COALESCE(
        count(*) FILTER (WHERE status_code = 'ERROR')::double precision /
            NULLIF(count(*), 0),
        0::double precision
    )::double precision AS error_rate,
    COALESCE(
        avg(extract(epoch FROM ended_at - started_at) * 1000000000)::bigint,
        0::bigint
    )::bigint AS average_latency
FROM spans
WHERE project_id = @project_id
  AND environment_id = @environment_id
  AND (sqlc.narg(from_time)::timestamptz IS NULL OR started_at >= sqlc.narg(from_time))
  AND (sqlc.narg(to_time)::timestamptz IS NULL OR started_at <= sqlc.narg(to_time))
GROUP BY service_name
ORDER BY service_name;

-- name: ListServiceDependencies :many
SELECT
    child.service_name,
    count(*) AS call_count
FROM spans AS child
JOIN spans AS parent
    ON parent.project_id = child.project_id
   AND parent.trace_id = child.trace_id
   AND parent.span_id = child.parent_span_id
WHERE child.project_id = @project_id
  AND child.environment_id = @environment_id
  AND parent.environment_id = @environment_id
  AND parent.service_name = @service_name
  AND parent.service_name <> child.service_name
GROUP BY child.service_name
ORDER BY call_count DESC, child.service_name;
