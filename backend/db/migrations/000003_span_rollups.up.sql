-- Derived data stays separate from telemetry. A dirty minute is always read
-- from spans until its replacement summary commits.
CREATE TABLE span_rollup_buckets (
    project_id uuid NOT NULL,
    environment_id uuid NOT NULL,
    bucket_start timestamptz NOT NULL,
    dirty boolean NOT NULL DEFAULT true,
    PRIMARY KEY (project_id, environment_id, bucket_start),
    FOREIGN KEY (project_id, environment_id) REFERENCES environments(project_id, id) ON DELETE CASCADE
);
CREATE INDEX span_rollup_buckets_pending_idx
    ON span_rollup_buckets (project_id, environment_id, bucket_start) WHERE dirty;

CREATE TABLE span_minute_rollups (
    project_id uuid NOT NULL,
    environment_id uuid NOT NULL,
    bucket_start timestamptz NOT NULL,
    service_name text NOT NULL,
    span_kind integer NOT NULL,
    endpoint text NOT NULL,
    entity_type text NOT NULL,
    entity_name text NOT NULL,
    calls bigint NOT NULL,
    errors bigint NOT NULL,
    duration_sum numeric NOT NULL,
    trace_ids text[] NOT NULL,
    request_started_at timestamptz[] NOT NULL,
    request_duration_ns bigint[] NOT NULL,
    request_failed boolean[] NOT NULL,
    FOREIGN KEY (project_id, environment_id, bucket_start)
        REFERENCES span_rollup_buckets(project_id, environment_id, bucket_start) ON DELETE CASCADE
);
CREATE INDEX span_minute_rollups_window_idx
    ON span_minute_rollups (project_id, environment_id, bucket_start);

CREATE FUNCTION span_request_endpoint(attributes jsonb, name text)
RETURNS text LANGUAGE sql STABLE PARALLEL SAFE AS $$
    SELECT CASE
        WHEN NULLIF(attributes->>'http.route', '') IS NOT NULL THEN
            concat_ws(' ', COALESCE(NULLIF(attributes->>'http.request.method', ''),
                NULLIF(attributes->>'http.method', '')), attributes->>'http.route')
        WHEN NULLIF(attributes->>'rpc.method', '') IS NOT NULL THEN
            concat_ws('/', NULLIF(attributes->>'rpc.service', ''), attributes->>'rpc.method')
        ELSE COALESCE(NULLIF(name, ''), '(unnamed)') END
$$;

CREATE FUNCTION span_request_failed(status text, attributes jsonb)
RETURNS boolean LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT status = 'ERROR' OR COALESCE(NULLIF(attributes->>'http.response.status_code', ''),
        attributes->>'http.status_code', '') ~ '^5[0-9]{2}$'
$$;

-- Existing telemetry is backfilled by the same bounded background work as new
-- arrivals. No percentile arrays are built during migration.
INSERT INTO span_rollup_buckets (project_id, environment_id, bucket_start)
SELECT DISTINCT project_id, environment_id,
    date_bin('1 minute', started_at, TIMESTAMPTZ '2000-01-01 00:00:00+00')
FROM spans;
