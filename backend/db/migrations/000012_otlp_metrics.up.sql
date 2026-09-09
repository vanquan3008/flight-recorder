-- OTLP Metrics ingestion: stores metric data points with their resource,
-- scope, and series identity. Only sum, gauge, and histogram instrument types
-- are stored; summary and exponential histogram are mapped to histogram.
CREATE TABLE metric_data_points (
    id               uuid PRIMARY KEY,
    project_id       uuid NOT NULL,
    environment_id   uuid NOT NULL,
    metric_name      text NOT NULL,
    metric_unit      text NOT NULL DEFAULT '',
    metric_type      text NOT NULL, -- 'gauge', 'sum', 'histogram'
    service_name     text NOT NULL,
    service_version  text,
    occurred_at      timestamptz NOT NULL,
    -- Numeric value for gauge and sum; NULL for histogram
    value            double precision,
    -- Histogram fields
    hist_count       bigint,
    hist_sum         double precision,
    hist_min         double precision,
    hist_max         double precision,
    hist_buckets     jsonb,          -- [{bound: float, count: int}, ...]
    -- Cumulative vs delta
    temporality      text NOT NULL DEFAULT 'cumulative',
    is_monotonic     boolean,
    -- Dimensions and metadata
    attributes       jsonb NOT NULL DEFAULT '{}'::jsonb,
    resource_attributes jsonb NOT NULL DEFAULT '{}'::jsonb,
    scope_name       text NOT NULL DEFAULT '',
    scope_version    text NOT NULL DEFAULT '',
    ingested_at      timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (project_id, environment_id) REFERENCES environments(project_id, id),
    UNIQUE (project_id, id),
    CONSTRAINT metric_data_points_attributes_object CHECK (jsonb_typeof(attributes) = 'object'),
    CONSTRAINT metric_data_points_resource_object   CHECK (jsonb_typeof(resource_attributes) = 'object'),
    CONSTRAINT metric_data_points_type_check CHECK (metric_type IN ('gauge', 'sum', 'histogram'))
);

CREATE INDEX metric_data_points_browse_idx
    ON metric_data_points (project_id, environment_id, metric_name, occurred_at DESC, id DESC);

CREATE INDEX metric_data_points_service_idx
    ON metric_data_points (project_id, environment_id, service_name, metric_name, occurred_at DESC);

CREATE INDEX metric_data_points_retention_idx
    ON metric_data_points (occurred_at, id);
