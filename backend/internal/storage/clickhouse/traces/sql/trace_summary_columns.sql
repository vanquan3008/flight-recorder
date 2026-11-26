
    s.trace_id AS id,
    min(s.started_at) AS trace_started,
    max(s.ended_at) AS trace_ended,
    toInt64(toUnixTimestamp64Nano(max(s.ended_at)) - toUnixTimestamp64Nano(min(s.started_at))) AS duration_nanos,
    argMin(s.service_name, tuple(s.parent_span_id != '', s.started_at, toString(s.event_id))) AS root_service,
    count() AS span_count,
    countIf(s.status_code = 'ERROR') AS error_count
