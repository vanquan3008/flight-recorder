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
