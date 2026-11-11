CREATE TABLE IF NOT EXISTS tracepilot_logs (
    project_id UUID,
    environment_id UUID,
    event_id UUID,
    occurred_at DateTime64(9, 'UTC') CODEC(Delta, ZSTD(1)),
    observed_at Nullable(DateTime64(9, 'UTC')),
    trace_id String,
    span_id String,
    trace_flags Int64,
    severity_number Int32,
    severity_text LowCardinality(String),
    body String CODEC(ZSTD(1)),
    event_name LowCardinality(String),
    service_name LowCardinality(String),
    service_version LowCardinality(String),
    resource_attributes String CODEC(ZSTD(1)),
    scope_name LowCardinality(String),
    scope_version LowCardinality(String),
    scope_attributes String CODEC(ZSTD(1)),
    attributes String CODEC(ZSTD(1)),
    INDEX trace_lookup trace_id TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX event_lookup event_id TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX body_search body TYPE ngrambf_v1(3, 32768, 3, 0) GRANULARITY 4
)
ENGINE = ReplacingMergeTree
PARTITION BY __PARTITION__
ORDER BY (project_id, environment_id, service_name, toStartOfMillisecond(occurred_at), event_id)
__TTL__
SETTINGS ttl_only_drop_parts = 1
