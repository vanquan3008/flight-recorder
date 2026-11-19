-- Spans, metric points and their rollups moved to ClickHouse. Databases created
-- from the earlier baseline drop them here; new databases never had them.
BEGIN;
DROP TABLE IF EXISTS span_minute_rollups, span_rollup_buckets, spans, metric_data_points, services, logs CASCADE;
DROP FUNCTION IF EXISTS span_is_dependency(jsonb, integer), span_dependency_type(jsonb), span_dependency_name(jsonb),
    span_request_endpoint(jsonb, text), span_request_failed(text, jsonb);
COMMIT;
