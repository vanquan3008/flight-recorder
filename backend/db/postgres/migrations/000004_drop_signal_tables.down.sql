-- Restores the empty signal tables the previous release expects. Dropped rows are not recovered.
BEGIN;

CREATE TABLE services (
    id              uuid PRIMARY KEY,
    project_id      uuid NOT NULL,
    environment_id  uuid NOT NULL,
    name            text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, environment_id) REFERENCES environments(project_id, id) ON DELETE CASCADE,
    UNIQUE (project_id, environment_id, name),
    UNIQUE (project_id, id)
);

CREATE TABLE spans (
    event_id                 uuid NOT NULL,
    project_id               uuid NOT NULL,
    environment_id           uuid NOT NULL,
    trace_id                 text NOT NULL,
    span_id                  text NOT NULL,
    parent_span_id           text,
    trace_state              text NOT NULL DEFAULT '',
    span_name                text NOT NULL,
    span_kind                integer NOT NULL,
    started_at               timestamptz NOT NULL,
    ended_at                 timestamptz NOT NULL,
    status_code              text NOT NULL,
    status_message           text NOT NULL DEFAULT '',
    flags                    bigint NOT NULL DEFAULT 0,
    service_name             text NOT NULL,
    service_version          text,
    resource_schema_url      text NOT NULL DEFAULT '',
    resource_attributes      jsonb NOT NULL DEFAULT '{}'::jsonb,
    scope_name               text NOT NULL DEFAULT '',
    scope_version            text NOT NULL DEFAULT '',
    scope_schema_url         text NOT NULL DEFAULT '',
    scope_attributes         jsonb NOT NULL DEFAULT '{}'::jsonb,
    span_attributes          jsonb NOT NULL DEFAULT '{}'::jsonb,
    span_events              jsonb NOT NULL DEFAULT '[]'::jsonb,
    span_links               jsonb NOT NULL DEFAULT '[]'::jsonb,
    dropped_attributes_count integer NOT NULL DEFAULT 0,
    dropped_events_count     integer NOT NULL DEFAULT 0,
    dropped_links_count      integer NOT NULL DEFAULT 0,
    PRIMARY KEY (event_id, started_at),
    FOREIGN KEY (project_id, environment_id) REFERENCES environments(project_id, id),
    UNIQUE (project_id, environment_id, trace_id, span_id, started_at),
    CONSTRAINT spans_resource_attributes_object CHECK (jsonb_typeof(resource_attributes) = 'object'),
    CONSTRAINT spans_scope_attributes_object CHECK (jsonb_typeof(scope_attributes) = 'object'),
    CONSTRAINT spans_span_attributes_object CHECK (jsonb_typeof(span_attributes) = 'object'),
    CONSTRAINT spans_events_array CHECK (jsonb_typeof(span_events) = 'array'),
    CONSTRAINT spans_links_array CHECK (jsonb_typeof(span_links) = 'array')
) PARTITION BY RANGE (started_at);

CREATE TABLE spans_default PARTITION OF spans DEFAULT;

CREATE INDEX spans_trace_time_idx
    ON spans (project_id, trace_id, started_at, event_id);

CREATE INDEX spans_service_time_idx
    ON spans (project_id, environment_id, service_name, started_at DESC, event_id DESC);

CREATE INDEX spans_window_idx
    ON spans (project_id, environment_id, started_at);

CREATE TABLE metric_data_points (
    id                  uuid NOT NULL,
    project_id          uuid NOT NULL,
    environment_id      uuid NOT NULL,
    metric_name         text NOT NULL,
    metric_unit         text NOT NULL DEFAULT '',
    metric_type         text NOT NULL, -- 'gauge', 'sum', 'histogram'
    service_name        text NOT NULL,
    service_version     text,
    occurred_at         timestamptz NOT NULL,
    -- Numeric value for gauge and sum; NULL for histogram
    value               double precision,
    -- Histogram fields
    hist_count          bigint,
    hist_sum            double precision,
    hist_min            double precision,
    hist_max            double precision,
    hist_buckets        jsonb,          -- [{bound: float, count: int}, ...]
    -- Cumulative vs delta
    temporality         text NOT NULL DEFAULT 'cumulative',
    is_monotonic        boolean,
    -- Dimensions and metadata
    attributes          jsonb NOT NULL DEFAULT '{}'::jsonb,
    resource_attributes jsonb NOT NULL DEFAULT '{}'::jsonb,
    scope_name          text NOT NULL DEFAULT '',
    scope_version       text NOT NULL DEFAULT '',
    ingested_at         timestamptz NOT NULL DEFAULT now(),
    exemplars           jsonb NOT NULL DEFAULT '[]'::jsonb,
    -- Metric start times distinguish counter intervals and resets.
    started_at          timestamptz,
    flags               bigint NOT NULL DEFAULT 0,
    scope_attributes    jsonb NOT NULL DEFAULT '{}',
    PRIMARY KEY (project_id, id, occurred_at),
    FOREIGN KEY (project_id, environment_id) REFERENCES environments(project_id, id),
    CONSTRAINT metric_data_points_attributes_object CHECK (jsonb_typeof(attributes) = 'object'),
    CONSTRAINT metric_data_points_resource_object   CHECK (jsonb_typeof(resource_attributes) = 'object'),
    CONSTRAINT metric_data_points_type_check CHECK (metric_type IN ('gauge', 'sum', 'histogram')),
    CONSTRAINT metric_data_points_exemplars_array CHECK (jsonb_typeof(exemplars) = 'array')
) PARTITION BY RANGE (occurred_at);

CREATE TABLE metric_data_points_default PARTITION OF metric_data_points DEFAULT;

CREATE INDEX metric_data_points_browse_idx
    ON metric_data_points (project_id, environment_id, metric_name, occurred_at DESC, id DESC);

CREATE INDEX metric_data_points_service_idx
    ON metric_data_points (project_id, environment_id, service_name, metric_name, occurred_at DESC);

CREATE FUNCTION span_is_dependency(span_attributes jsonb, span_kind integer)
RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$
    SELECT span_attributes ? 'db.system'
        OR span_attributes ? 'db.system.name'
        OR span_attributes ? 'messaging.system'
        OR span_kind = 3
$$;

CREATE FUNCTION span_dependency_type(span_attributes jsonb)
RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$
    SELECT CASE
        WHEN span_attributes ? 'db.system' OR span_attributes ? 'db.system.name' THEN 'database'
        WHEN span_attributes ? 'messaging.system' THEN 'queue'
        ELSE 'external'
    END
$$;

CREATE FUNCTION span_dependency_name(span_attributes jsonb)
RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$
    SELECT CASE
        WHEN span_attributes ? 'db.system' OR span_attributes ? 'db.system.name' THEN
            COALESCE(
                NULLIF(span_attributes->>'db.namespace', ''),
                NULLIF(span_attributes->>'db.name', ''),
                NULLIF(span_attributes->>'server.address', ''),
                NULLIF(span_attributes->>'db.system.name', ''),
                span_attributes->>'db.system')
        WHEN span_attributes ? 'messaging.system' THEN
            COALESCE(
                NULLIF(span_attributes->>'messaging.destination.name', ''),
                NULLIF(span_attributes->>'messaging.destination', ''),
                span_attributes->>'messaging.system')
        ELSE
            COALESCE(
                NULLIF(span_attributes->>'peer.service', ''),
                NULLIF(span_attributes->>'server.address', ''),
                NULLIF(span_attributes->>'http.host', ''),
                NULLIF(span_attributes->>'url.full', ''))
    END
$$;

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

COMMIT;
