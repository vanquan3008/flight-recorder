CREATE TABLE IF NOT EXISTS tracepilot_span_minutes (
    project_id UUID,
    environment_id UUID,
    minute DateTime('UTC'),
    service_name LowCardinality(String),
    span_kind Int32,
    endpoint String CODEC(ZSTD(1)),
    dependency_type LowCardinality(String),
    dependency_name String CODEC(ZSTD(1)),
    calls UInt64,
    errors UInt64,
    failed UInt64,
    duration_sum Int64,
    duration_quantiles AggregateFunction(quantilesTDigest(0.95, 0.99), Int64),
    computed_at DateTime64(3, 'UTC')
)
ENGINE = ReplacingMergeTree(computed_at)
PARTITION BY toStartOfWeek(minute, 1, 'UTC')
ORDER BY (project_id, environment_id, minute, service_name, span_kind, endpoint, dependency_type, dependency_name)
