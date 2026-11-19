CREATE TABLE IF NOT EXISTS tracepilot_metric_points (
    project_id UUID,
    environment_id UUID,
    id UUID,
    series_id String,
    metric_name LowCardinality(String),
    metric_unit LowCardinality(String),
    metric_type LowCardinality(String),
    service_name LowCardinality(String),
    service_version LowCardinality(String),
    occurred_at DateTime64(9, 'UTC') CODEC(Delta, ZSTD(1)),
    started_at Nullable(DateTime64(9, 'UTC')),
    flags UInt32,
    value Nullable(Float64) CODEC(Gorilla, ZSTD(1)),
    hist_count Nullable(Int64),
    hist_sum Nullable(Float64),
    hist_min Nullable(Float64),
    hist_max Nullable(Float64),
    hist_buckets String CODEC(ZSTD(1)),
    temporality LowCardinality(String),
    is_monotonic Nullable(Bool),
    attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    resource_attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    scope_name LowCardinality(String),
    scope_version LowCardinality(String),
    scope_attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    exemplars String CODEC(ZSTD(1)),
    INDEX attribute_keys mapKeys(attributes) TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX attribute_values mapValues(attributes) TYPE bloom_filter(0.01) GRANULARITY 1
)
ENGINE = ReplacingMergeTree
PARTITION BY toStartOfWeek(occurred_at, 1, 'UTC')
ORDER BY (project_id, environment_id, metric_name, service_name, series_id, occurred_at, id)
