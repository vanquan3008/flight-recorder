-- A trace matches when any of its spans matches the filters; the summary then
-- covers every span of that trace. The summary scan is bounded to one hour
-- either side of the matching spans: unbounded, it probed every daily
-- partition (including the empty future ones) once per matching trace. Spans of
-- a trace that start more than an hour from every matching span are left out of
-- its summary.
-- name: ListTraces :many
WITH matching_traces AS (
    SELECT spans.trace_id, min(spans.started_at) AS first_match, max(spans.started_at) AS last_match FROM spans
    WHERE spans.project_id = @project_id
      AND spans.environment_id = @environment_id
      AND (sqlc.narg(service)::text IS NULL OR spans.service_name = sqlc.narg(service))
      AND (sqlc.narg(operation)::text IS NULL OR spans.span_name = sqlc.narg(operation))
      AND (sqlc.narg(status)::text IS NULL OR spans.status_code = sqlc.narg(status))
      AND (sqlc.narg(trace_id)::text IS NULL OR spans.trace_id = sqlc.narg(trace_id))
      AND (sqlc.narg(from_time)::timestamptz IS NULL OR spans.started_at >= sqlc.narg(from_time))
      AND (sqlc.narg(to_time)::timestamptz IS NULL OR spans.started_at <= sqlc.narg(to_time))
    GROUP BY spans.trace_id
), match_bounds AS (
    SELECT min(matching_traces.first_match) - interval '1 hour' AS earliest,
           max(matching_traces.last_match) + interval '1 hour' AS latest
    FROM matching_traces
)
SELECT spans.trace_id,
       min(spans.started_at)::timestamptz AS started_at,
       max(spans.ended_at)::timestamptz AS ended_at,
       max(spans.started_at)::timestamptz AS cursor_at,
       (extract(epoch FROM max(spans.ended_at) - min(spans.started_at)) * 1000000000)::bigint AS duration_nanos,
       (array_agg(spans.service_name ORDER BY (NULLIF(spans.parent_span_id, '') IS NULL) DESC, spans.started_at, spans.event_id))[1]::text AS root_service,
       count(*) AS span_count,
       count(*) FILTER (WHERE spans.status_code = 'ERROR') AS error_count
FROM spans
CROSS JOIN match_bounds
WHERE spans.project_id = @project_id
  AND spans.environment_id = @environment_id
  AND spans.started_at >= match_bounds.earliest
  AND spans.started_at <= match_bounds.latest
  AND spans.trace_id IN (SELECT matching_traces.trace_id FROM matching_traces)
GROUP BY spans.trace_id
HAVING (sqlc.narg(before)::timestamptz IS NULL
        OR max(spans.started_at) < sqlc.narg(before)
        OR (max(spans.started_at) = sqlc.narg(before) AND spans.trace_id < sqlc.narg(cursor_id)::text))
   AND (sqlc.narg(min_duration_ns)::bigint IS NULL
        OR (extract(epoch FROM max(spans.ended_at) - min(spans.started_at)) * 1000000000)::bigint >= sqlc.narg(min_duration_ns))
   AND (sqlc.narg(max_duration_ns)::bigint IS NULL
        OR (extract(epoch FROM max(spans.ended_at) - min(spans.started_at)) * 1000000000)::bigint <= sqlc.narg(max_duration_ns))
ORDER BY max(spans.started_at) DESC, spans.trace_id DESC
LIMIT sqlc.arg(row_limit)::integer;

-- name: ListLogs :many
SELECT event_id, occurred_at, observed_at, COALESCE(trace_id, '')::text AS trace_id, COALESCE(span_id, '')::text AS span_id,
       trace_flags, severity_number, severity_text, COALESCE(body, 'null'::jsonb)::jsonb AS body, event_name, service_name,
       COALESCE(service_version, '')::text AS service_version, resource_attributes, scope_name, scope_version,
       scope_attributes, log_attributes
FROM logs
WHERE project_id = @project_id
  AND environment_id = @environment_id
  AND (sqlc.narg(service)::text IS NULL OR service_name = sqlc.narg(service))
  AND (sqlc.narg(severity)::text IS NULL OR severity_text = sqlc.narg(severity))
  AND (sqlc.narg(trace_id)::text IS NULL OR trace_id = sqlc.narg(trace_id))
  AND (sqlc.narg(span_id)::text IS NULL OR span_id = sqlc.narg(span_id))
  AND (sqlc.narg(search)::text IS NULL OR body::text ILIKE '%' || sqlc.narg(search) || '%')
  AND (sqlc.narg(from_time)::timestamptz IS NULL OR occurred_at >= sqlc.narg(from_time))
  AND (sqlc.narg(to_time)::timestamptz IS NULL OR occurred_at <= sqlc.narg(to_time))
  AND (sqlc.narg(before)::timestamptz IS NULL
       OR occurred_at < sqlc.narg(before)
       OR (occurred_at = sqlc.narg(before) AND event_id < sqlc.narg(cursor_id)::uuid))
ORDER BY occurred_at DESC, event_id DESC
LIMIT sqlc.arg(row_limit)::integer;

-- name: GetLog :one
SELECT event_id, occurred_at, observed_at, COALESCE(trace_id, '')::text AS trace_id, COALESCE(span_id, '')::text AS span_id,
       trace_flags, severity_number, severity_text, COALESCE(body, 'null'::jsonb)::jsonb AS body, event_name, service_name,
       COALESCE(service_version, '')::text AS service_version, resource_attributes, scope_name, scope_version,
       scope_attributes, log_attributes
FROM logs
WHERE project_id = @project_id AND environment_id = @environment_id AND event_id = @event_id;

-- One statement keeps summary, percentiles and breakdowns on the same snapshot.
-- Endpoints use the route or RPC method when present, else the span name.
-- name: RequestPerformance :many
WITH inbound AS (
    SELECT service_name, started_at,
        CASE
            WHEN NULLIF(span_attributes->>'http.route', '') IS NOT NULL THEN
                concat_ws(' ', COALESCE(NULLIF(span_attributes->>'http.request.method', ''),
                    NULLIF(span_attributes->>'http.method', '')), span_attributes->>'http.route')
            WHEN NULLIF(span_attributes->>'rpc.method', '') IS NOT NULL THEN
                concat_ws('/', NULLIF(span_attributes->>'rpc.service', ''), span_attributes->>'rpc.method')
            ELSE COALESCE(NULLIF(span_name, ''), '(unnamed)')
        END AS endpoint,
        extract(epoch FROM ended_at - started_at) * 1000000000 AS duration_ns,
        (status_code = 'ERROR' OR COALESCE(NULLIF(span_attributes->>'http.response.status_code', ''),
            span_attributes->>'http.status_code', '') ~ '^5[0-9]{2}$') AS failed
    FROM spans
    WHERE project_id = @project_id
      AND environment_id = @environment_id
      AND span_kind = 2
      AND started_at >= @from_time::timestamptz
      AND started_at < @to_time::timestamptz
      AND (sqlc.narg(service)::text IS NULL OR service_name = sqlc.narg(service))
), observed AS (
    SELECT inbound.*,
        floor(extract(epoch FROM inbound.started_at - @from_time::timestamptz) / @bucket_seconds::integer)::integer AS bucket
    FROM inbound
    WHERE sqlc.narg(endpoint)::text IS NULL OR inbound.endpoint = sqlc.narg(endpoint)
), grouped AS (
    SELECT
        CASE WHEN grouping(bucket) = 0 THEN 'bucket'
             WHEN grouping(endpoint) = 0 THEN 'endpoint'
             WHEN grouping(service_name) = 0 THEN 'service'
             ELSE 'summary' END AS dimension,
        COALESCE(service_name, '') AS service,
        COALESCE(endpoint, '') AS endpoint,
        COALESCE(bucket, -1) AS bucket,
        count(*) AS requests,
        count(*) FILTER (WHERE failed) AS errors,
        COALESCE(avg(duration_ns), 0)::bigint AS average_ns,
        COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ns), 0)::bigint AS p95_ns,
        COALESCE(percentile_cont(0.99) WITHIN GROUP (ORDER BY duration_ns), 0)::bigint AS p99_ns
    FROM observed
    GROUP BY GROUPING SETS ((), (service_name), (service_name, endpoint), (bucket))
), ranked AS (
    SELECT grouped.*,
        row_number() OVER (PARTITION BY grouped.dimension ORDER BY grouped.requests DESC, grouped.service, grouped.endpoint) AS position
    FROM grouped
)
SELECT ranked.dimension::text AS dimension, ranked.service::text AS service, ranked.endpoint::text AS endpoint,
       ranked.bucket::integer AS bucket, ranked.requests::bigint AS requests, ranked.errors::bigint AS errors,
       ranked.average_ns::bigint AS average_ns, ranked.p95_ns::bigint AS p95_ns, ranked.p99_ns::bigint AS p99_ns
FROM ranked
WHERE ranked.dimension IN ('summary', 'bucket') OR ranked.position <= sqlc.arg(row_limit)::integer
ORDER BY ranked.dimension, ranked.position;

-- One page of a trace's spans, oldest first, after the (started_at, event_id)
-- cursor. The caller asks for one row more than it will return; `fetched` lets it
-- tell whether anything lies beyond the page. A page also ends once the spans'
-- serialized JSON passes @max_bytes (measured on the text form, because stored
-- jsonb is compressed and understates what the response will weigh), but always holds at least one span so paging
-- advances even when a single span is larger than the budget.
-- name: ListTraceSpans :many
WITH page AS (
    SELECT spans.*,
        (octet_length(spans.resource_attributes::text) + octet_length(spans.scope_attributes::text)
         + octet_length(spans.span_attributes::text) + octet_length(spans.span_events::text)
         + octet_length(spans.span_links::text))::bigint AS payload_bytes
    FROM spans
    WHERE spans.project_id = @project_id
      AND spans.environment_id = @environment_id
      AND spans.trace_id = @trace_id
      AND (sqlc.narg(after_started_at)::timestamptz IS NULL
           OR spans.started_at > sqlc.narg(after_started_at)
           OR (spans.started_at = sqlc.narg(after_started_at) AND spans.event_id > sqlc.narg(after_event_id)::uuid))
    ORDER BY spans.started_at, spans.event_id
    LIMIT sqlc.arg(row_limit)::integer
), sized AS (
    SELECT page.*,
        count(*) OVER () AS fetched,
        row_number() OVER (ORDER BY page.started_at, page.event_id) AS position,
        sum(page.payload_bytes) OVER (ORDER BY page.started_at, page.event_id) AS running_bytes
    FROM page
)
SELECT
    sized.event_id::text AS event_id,
    sized.trace_id,
    sized.span_id,
    COALESCE(sized.parent_span_id, '')::text AS parent_span_id,
    sized.trace_state,
    sized.span_name,
    sized.span_kind,
    sized.started_at,
    sized.ended_at,
    sized.status_code,
    sized.status_message,
    sized.flags,
    sized.service_name,
    COALESCE(sized.service_version, '')::text AS service_version,
    sized.resource_attributes,
    sized.scope_name,
    sized.scope_version,
    sized.scope_attributes,
    sized.span_attributes,
    sized.span_events,
    sized.span_links,
    sized.fetched::bigint AS fetched
FROM sized
WHERE sized.position = 1 OR sized.running_bytes <= sqlc.arg(max_bytes)::bigint
ORDER BY sized.started_at, sized.event_id;
