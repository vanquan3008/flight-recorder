package postgres

import (
	"context"
	"fmt"
)

func (database *Database) ListTraces(ctx context.Context, projectID string, filter TraceFilter) ([]TraceSummary, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT trace_id, min(started_at), max(ended_at), max(started_at),
			(extract(epoch FROM max(ended_at) - min(started_at)) * 1000000000)::bigint,
			(array_agg(service_name ORDER BY started_at, event_id))[1], count(*),
			count(*) FILTER (WHERE status_code = 'ERROR')
		FROM spans
		WHERE project_id = $1 AND environment_id = $2 AND ($3 = '' OR service_name = $3)
			AND ($4 = '' OR span_name = $4) AND ($5 = '' OR status_code = $5)
			AND ($6 = '' OR trace_id = $6) AND ($7::timestamptz IS NULL OR started_at >= $7)
			AND ($8::timestamptz IS NULL OR started_at <= $8)
		GROUP BY trace_id
		HAVING $9::timestamptz IS NULL OR max(started_at) < $9 OR (max(started_at) = $9 AND trace_id < $10)
		ORDER BY max(started_at) DESC, trace_id DESC LIMIT $11`,
		projectID, filter.EnvironmentID, filter.Service, filter.Operation, filter.Status, filter.TraceID,
		nullableTime(filter.From), nullableTime(filter.To), nullableTime(filter.Before), filter.CursorID, filter.Limit+1)
	if queryError != nil {
		return nil, fmt.Errorf("list traces: %w", queryError)
	}
	defer rows.Close()
	traces := make([]TraceSummary, 0, filter.Limit+1)
	for rows.Next() {
		var trace TraceSummary
		if scanError := rows.Scan(&trace.TraceID, &trace.StartedAt, &trace.EndedAt, &trace.CursorAt, &trace.DurationNanos, &trace.RootService, &trace.SpanCount, &trace.ErrorCount); scanError != nil {
			return nil, fmt.Errorf("scan trace: %w", scanError)
		}
		traces = append(traces, trace)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate traces: %w", rowsError)
	}
	return traces, nil
}

func (database *Database) GetTrace(ctx context.Context, projectID, environmentID, traceID string) ([]SpanRecord, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT event_id, trace_id, span_id, COALESCE(parent_span_id, ''), trace_state,
			span_name, span_kind, started_at, ended_at, status_code, status_message, flags,
			service_name, COALESCE(service_version, ''), resource_attributes,
			scope_name, scope_version, scope_attributes, span_attributes, span_events, span_links
		FROM spans WHERE project_id = $1 AND environment_id = $2 AND trace_id = $3 ORDER BY started_at, event_id`, projectID, environmentID, traceID)
	if queryError != nil {
		return nil, fmt.Errorf("get trace: %w", queryError)
	}
	defer rows.Close()
	spans := make([]SpanRecord, 0)
	for rows.Next() {
		var span SpanRecord
		if scanError := rows.Scan(&span.EventID, &span.TraceID, &span.SpanID, &span.ParentSpanID, &span.TraceState, &span.Name, &span.Kind, &span.StartedAt, &span.EndedAt, &span.StatusCode, &span.StatusMessage, &span.Flags, &span.ServiceName, &span.ServiceVersion, &span.ResourceAttributes, &span.ScopeName, &span.ScopeVersion, &span.ScopeAttributes, &span.Attributes, &span.Events, &span.Links); scanError != nil {
			return nil, fmt.Errorf("scan span: %w", scanError)
		}
		spans = append(spans, span)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate spans: %w", rowsError)
	}
	return spans, nil
}
