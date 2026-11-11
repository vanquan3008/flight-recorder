-- Initial PostgreSQL baseline. Later schema changes need a new migration version.
BEGIN;

-- identity
CREATE TABLE projects (
    id          uuid PRIMARY KEY,
    slug        text NOT NULL UNIQUE,
    name        text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT projects_slug_format CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,62}$')
);

CREATE TABLE environments (
    id          uuid PRIMARY KEY,
    project_id  uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, name),
    UNIQUE (project_id, id)
);

CREATE TABLE users (
    id                    uuid PRIMARY KEY,
    email                 text NOT NULL,
    display_name          text NOT NULL,
    password_hash         text NOT NULL,
    platform_admin        boolean NOT NULL DEFAULT false,
    must_change_password  boolean NOT NULL DEFAULT false,
    disabled_at           timestamptz,
    tokens_valid_after    timestamptz NOT NULL DEFAULT to_timestamp(0),
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    last_login_at         timestamptz,
    CONSTRAINT users_email_format CHECK (email = lower(email) AND length(email) <= 254 AND email ~ '^[^@[:space:]]+@[^@[:space:]]+$'),
    CONSTRAINT users_display_name_length CHECK (length(btrim(display_name)) BETWEEN 1 AND 100)
);

CREATE UNIQUE INDEX users_email_key ON users (email);

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

-- A project has owners (who made it, and run it) and users (who were allowed to see its telemetry).
CREATE TABLE project_members (
    project_id  uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, user_id),
    CONSTRAINT project_members_role CHECK (role IN ('owner', 'user'))
);

CREATE INDEX project_members_user_idx ON project_members (user_id);

-- Refresh sessions. Only a digest of the opaque refresh token is stored, and each use replaces it.
CREATE TABLE sessions (
    token_hash      bytea PRIMARY KEY,
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at      timestamptz NOT NULL DEFAULT now(),
    last_seen_at    timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    max_expires_at  timestamptz NOT NULL,
    user_agent      text NOT NULL DEFAULT '',
    CONSTRAINT sessions_expiry_order CHECK (expires_at <= max_expires_at)
);

CREATE INDEX sessions_user_idx ON sessions (user_id);

CREATE INDEX sessions_expires_idx ON sessions (max_expires_at);

-- Credentials for telemetry senders. The token itself is a signed JWT shown once; this row is what
-- lets an owner list and revoke it.
CREATE TABLE ingest_tokens (
    id          uuid PRIMARY KEY,
    project_id  uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        text NOT NULL,
    created_by  uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL,
    revoked_at  timestamptz,
    UNIQUE (project_id, name),
    CONSTRAINT ingest_tokens_name_format CHECK (name ~ '^[A-Za-z0-9][A-Za-z0-9 ._-]{0,63}$')
);

-- Who did what in the control plane: accounts, projects, membership and ingest tokens.
CREATE TABLE audit_events (
    id             uuid PRIMARY KEY,
    project_id     uuid REFERENCES projects(id) ON DELETE CASCADE,
    actor_user_id  uuid REFERENCES users(id) ON DELETE SET NULL,
    action         text NOT NULL,
    target_type    text NOT NULL,
    target_id      text NOT NULL,
    detail         jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT audit_events_detail_object CHECK (jsonb_typeof(detail) = 'object')
);

CREATE INDEX audit_events_project_idx ON audit_events (project_id, created_at DESC, id DESC);

CREATE INDEX audit_events_created_idx ON audit_events (created_at);

-- telemetry
-- Canonical telemetry and signal tables, indexes and default partitions.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE events (
    id               uuid NOT NULL,
    project_id       uuid NOT NULL,
    environment_id   uuid NOT NULL,
    service_name     text NOT NULL,
    service_version  text,
    trace_id         text,
    span_id          text,
    parent_span_id   text,
    request_id       text,
    event_type       text NOT NULL,
    occurred_at      timestamptz NOT NULL,
    duration_ns      bigint CHECK (duration_ns IS NULL OR duration_ns >= 0),
    severity         text,
    status           text,
    http_status      integer CHECK (http_status IS NULL OR http_status BETWEEN 100 AND 599),
    operation        text,
    error_type       text,
    error_message    text,
    attributes       jsonb NOT NULL DEFAULT '{}'::jsonb,
    ingested_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, occurred_at),
    FOREIGN KEY (project_id, environment_id) REFERENCES environments(project_id, id),
    CONSTRAINT events_attributes_object CHECK (jsonb_typeof(attributes) = 'object')
) PARTITION BY RANGE (occurred_at);

CREATE TABLE events_default PARTITION OF events DEFAULT;

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

-- Detection, topology and quality queries read one environment's spans in a
-- recent window. Without a time-led index the planner read the whole daily
-- partition to find a 5 to 35 minute slice. Measured on 2M spans: detection
-- window 28 ms -> 12 ms, quality check 100 ms -> 31 ms, about 18% slower span
-- inserts. A BRIN index was tried and lost (coarse ranges, slower than a scan).
CREATE INDEX spans_window_idx
    ON spans (project_id, environment_id, started_at);

CREATE TABLE logs (
    event_id                 uuid NOT NULL,
    project_id               uuid NOT NULL,
    environment_id           uuid NOT NULL,
    occurred_at              timestamptz NOT NULL,
    observed_at              timestamptz,
    trace_id                 text,
    span_id                  text,
    trace_flags              bigint NOT NULL DEFAULT 0,
    severity_number          integer NOT NULL DEFAULT 0,
    severity_text            text NOT NULL DEFAULT '',
    body                     jsonb,
    event_name               text NOT NULL DEFAULT '',
    service_name             text NOT NULL,
    service_version          text,
    resource_schema_url      text NOT NULL DEFAULT '',
    resource_attributes      jsonb NOT NULL DEFAULT '{}'::jsonb,
    scope_name               text NOT NULL DEFAULT '',
    scope_version            text NOT NULL DEFAULT '',
    scope_schema_url         text NOT NULL DEFAULT '',
    scope_attributes         jsonb NOT NULL DEFAULT '{}'::jsonb,
    log_attributes           jsonb NOT NULL DEFAULT '{}'::jsonb,
    dropped_attributes_count integer NOT NULL DEFAULT 0,
    PRIMARY KEY (event_id, occurred_at),
    FOREIGN KEY (project_id, environment_id) REFERENCES environments(project_id, id),
    CONSTRAINT logs_resource_attributes_object CHECK (jsonb_typeof(resource_attributes) = 'object'),
    CONSTRAINT logs_scope_attributes_object CHECK (jsonb_typeof(scope_attributes) = 'object'),
    CONSTRAINT logs_attributes_object CHECK (jsonb_typeof(log_attributes) = 'object')
) PARTITION BY RANGE (occurred_at);

CREATE TABLE logs_default PARTITION OF logs DEFAULT;

CREATE INDEX logs_time_idx
    ON logs (project_id, environment_id, occurred_at DESC, event_id DESC);

CREATE INDEX logs_trace_idx
    ON logs (project_id, trace_id, occurred_at, event_id) WHERE trace_id IS NOT NULL;

CREATE INDEX logs_body_trgm_idx ON logs USING GIN (
    (body::text) gin_trgm_ops
) WHERE body IS NOT NULL;

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

-- incidents
-- Incidents, investigation history, evidence and replay executions.

CREATE TABLE incidents (
    id                uuid PRIMARY KEY,
    project_id        uuid NOT NULL,
    environment_id    uuid NOT NULL,
    primary_service   text NOT NULL,
    trace_id          text,
    request_id        text,
    fingerprint       text NOT NULL,
    title             text NOT NULL,
    error_type        text,
    error_message     text,
    http_status       integer CHECK (http_status IS NULL OR http_status BETWEEN 100 AND 599),
    severity          text NOT NULL,
    status            text NOT NULL,
    first_seen_at     timestamptz NOT NULL,
    last_seen_at      timestamptz NOT NULL,
    occurrence_count  bigint NOT NULL DEFAULT 1 CHECK (occurrence_count > 0),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, environment_id) REFERENCES environments(project_id, id),
    UNIQUE (project_id, id),
    CONSTRAINT incidents_time_order CHECK (last_seen_at >= first_seen_at),
    CONSTRAINT incidents_status_check CHECK (status IN ('OPEN', 'INVESTIGATING', 'RESOLVED', 'IGNORED')),
    CONSTRAINT incidents_severity_check CHECK (severity IN ('ERROR', 'CRITICAL'))
);

CREATE INDEX incidents_browse_idx
    ON incidents (project_id, environment_id, last_seen_at DESC, id DESC);

CREATE INDEX incidents_status_idx
    ON incidents (project_id, status, severity, last_seen_at DESC);

CREATE INDEX incidents_fingerprint_idx
    ON incidents (project_id, environment_id, fingerprint, last_seen_at DESC);

-- incident_occurrences and incident_events reference events by id without a
-- foreign key. Retention drops old event partitions, so these rows may outlive
-- their event; readers must join with an inner join and tolerate the gap.
CREATE TABLE incident_occurrences (
    id           uuid PRIMARY KEY,
    project_id   uuid NOT NULL,
    incident_id  uuid NOT NULL,
    event_id     uuid NOT NULL,
    occurred_at  timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, incident_id) REFERENCES incidents(project_id, id) ON DELETE CASCADE,
    UNIQUE (incident_id, event_id)
);

-- events keeps every signal but has no secondary indexes on purpose. Reads go
-- through spans, logs and metric_data_points; events is only joined by
-- (project_id, id) for incident evidence and counted by time for retention.
-- Four indexes that no query used cost about 29% of insert throughput.
CREATE INDEX incident_occurrences_time_idx
    ON incident_occurrences (incident_id, occurred_at DESC, id DESC);

CREATE UNIQUE INDEX incident_occurrences_project_event_idx
    ON incident_occurrences (project_id, event_id);

CREATE TABLE incident_events (
    project_id   uuid NOT NULL,
    incident_id  uuid NOT NULL,
    event_id     uuid NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (incident_id, event_id),
    FOREIGN KEY (project_id, incident_id) REFERENCES incidents(project_id, id) ON DELETE CASCADE
);

CREATE TABLE incident_status_changes (
    id              uuid PRIMARY KEY,
    project_id      uuid NOT NULL,
    incident_id     uuid NOT NULL,
    previous_status text NOT NULL,
    status          text NOT NULL,
    changed_at      timestamptz NOT NULL,
    actor_user_id   uuid REFERENCES users(id) ON DELETE SET NULL,
    FOREIGN KEY (project_id, incident_id) REFERENCES incidents(project_id, id) ON DELETE CASCADE,
    CONSTRAINT incident_status_changes_status CHECK (status IN ('OPEN', 'INVESTIGATING', 'RESOLVED', 'IGNORED')),
    CONSTRAINT incident_status_changes_previous_status CHECK (previous_status IN ('OPEN', 'INVESTIGATING', 'RESOLVED', 'IGNORED'))
);

CREATE INDEX incident_status_changes_timeline_idx
    ON incident_status_changes (project_id, incident_id, changed_at DESC, id DESC);

CREATE TABLE incident_evidence (
    id                  uuid PRIMARY KEY,
    project_id          uuid NOT NULL,
    environment_id      uuid NOT NULL,
    incident_id         uuid NOT NULL,
    signal_type         text NOT NULL,
    entity_type         text NOT NULL,
    entity_id           text NOT NULL,
    metric_name         text NOT NULL,
    direction           text NOT NULL CHECK (direction IN ('increase', 'decrease', 'event')),
    observed_value      double precision NOT NULL,
    baseline_value      double precision NOT NULL,
    change_ratio        double precision NOT NULL,
    window_started_at   timestamptz NOT NULL,
    window_ended_at     timestamptz NOT NULL,
    trace_ids           text[] NOT NULL DEFAULT '{}',
    summary             text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    series_id           text NOT NULL DEFAULT '',
    FOREIGN KEY (project_id, environment_id) REFERENCES environments(project_id, id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, incident_id) REFERENCES incidents(project_id, id) ON DELETE CASCADE,
    CONSTRAINT incident_evidence_signal_series_unique
        UNIQUE (project_id, incident_id, signal_type, entity_id, metric_name, series_id, window_ended_at),
    CONSTRAINT incident_evidence_window_order CHECK (window_ended_at >= window_started_at)
);

CREATE INDEX incident_evidence_timeline_idx
    ON incident_evidence (project_id, environment_id, incident_id, window_ended_at DESC, id DESC);

CREATE TABLE replay_executions (
    id                  uuid PRIMARY KEY,
    project_id          uuid NOT NULL,
    incident_id         uuid NOT NULL,
    target              text NOT NULL,
    status              text NOT NULL,
    started_at          timestamptz,
    completed_at        timestamptz,
    duration_ns         bigint CHECK (duration_ns IS NULL OR duration_ns >= 0),
    http_status         integer CHECK (http_status IS NULL OR http_status BETWEEN 100 AND 599),
    comparison          jsonb,
    created_by          text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, incident_id) REFERENCES incidents(project_id, id) ON DELETE CASCADE
);

CREATE INDEX replay_executions_incident_idx
    ON replay_executions (incident_id, created_at DESC, id DESC);

-- notifications
-- Webhook configuration and durable delivery outbox.

-- Webhook notification delivery: operators register HTTPS endpoints and select
-- which event types trigger a delivery attempt. Deliveries are retried up to
-- max_attempts times by the durable notification worker.
CREATE TABLE webhooks (
    id           uuid PRIMARY KEY,
    project_id   uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name         text NOT NULL,
    url          text NOT NULL,
    secret       text NOT NULL,          -- HMAC-SHA256 signing secret (protected database value)
    events       text[] NOT NULL,        -- e.g. ['incident.opened']
    enabled      bool NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, name),
    UNIQUE (project_id, id),
    CONSTRAINT webhooks_url_format   CHECK (url   LIKE 'https://%'),
    CONSTRAINT webhooks_events_nonempty CHECK (cardinality(events) > 0),
    CONSTRAINT webhooks_events_supported CHECK (
        array_ndims(events) = 1
        AND events <@ ARRAY['incident.opened', 'incident.resolved']::text[]
    )
);

CREATE INDEX webhooks_project_idx ON webhooks (project_id, enabled);

CREATE TABLE webhook_deliveries (
    id             uuid PRIMARY KEY,
    project_id     uuid NOT NULL,
    webhook_id     uuid NOT NULL,
    event_type     text NOT NULL,
    payload        jsonb NOT NULL,
    status         text NOT NULL DEFAULT 'pending',  -- pending, sending, succeeded, failed
    attempt        integer NOT NULL DEFAULT 0,
    max_attempts   integer NOT NULL DEFAULT 3,
    http_status    integer,
    last_error     text,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, webhook_id) REFERENCES webhooks(project_id, id) ON DELETE CASCADE,
    UNIQUE (project_id, id),
    CONSTRAINT webhook_delivery_status CHECK (status IN ('pending', 'sending', 'succeeded', 'failed'))
);

CREATE INDEX webhook_deliveries_pending_idx
    ON webhook_deliveries (project_id, next_attempt_at)
    WHERE status = 'pending';

CREATE INDEX webhook_deliveries_webhook_idx
    ON webhook_deliveries (project_id, webhook_id, created_at DESC, id DESC);

-- span_analysis
-- Shared SQL functions for span classification and request analysis.

-- Dependency identity of a span, shared by topology and detection queries so
-- both name the same database, queue or external target. Single-statement
-- IMMUTABLE SQL functions are inlined by the planner, so they cost nothing
-- per row compared with writing the expressions in each query.

-- A span calls a dependency when it is a database or messaging operation, or
-- any CLIENT span (kind 3).
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

-- span_rollups
-- Derived minute summaries and bounded backfill scheduling.

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

-- Existing telemetry is backfilled by the same bounded background work as new
-- arrivals. No percentile arrays are built during migration.
INSERT INTO span_rollup_buckets (project_id, environment_id, bucket_start)
SELECT DISTINCT project_id, environment_id,
    date_bin('1 minute', started_at, TIMESTAMPTZ '2000-01-01 00:00:00+00')
FROM spans;

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
