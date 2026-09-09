package postgres

import (
	"context"
	"fmt"

	"github.com/vanquan3008/tracepilot/internal/query"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

func (database *Database) ListTraces(ctx context.Context, projectID string, filter query.TraceFilter) ([]query.TraceSummary, error) {
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
		HAVING ($9::timestamptz IS NULL OR max(started_at) < $9 OR (max(started_at) = $9 AND trace_id < $10))
		   AND ($11::bigint IS NULL OR (extract(epoch FROM max(ended_at) - min(started_at)) * 1000000000)::bigint >= $11)
		   AND ($12::bigint IS NULL OR (extract(epoch FROM max(ended_at) - min(started_at)) * 1000000000)::bigint <= $12)
		ORDER BY max(started_at) DESC, trace_id DESC LIMIT $13`,
		projectID, filter.EnvironmentID, filter.Service, filter.Operation, filter.Status, filter.TraceID,
		nullableTime(filter.From), nullableTime(filter.To), nullableTime(filter.Before), nullableString(filter.CursorID), nullableInt64Ptr(filter.MinDurationNs), nullableInt64Ptr(filter.MaxDurationNs), filter.Limit+1)
	if queryError != nil {
		return nil, fmt.Errorf("list traces: %w", queryError)
	}
	defer rows.Close()
	traces := make([]query.TraceSummary, 0, filter.Limit+1)
	for rows.Next() {
		var trace query.TraceSummary
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

func (database *Database) GetTrace(ctx context.Context, projectID, environmentID, traceID string) ([]query.SpanRecord, error) {
	project, environment, scopeError := sqlcScope(projectID, environmentID)
	if scopeError != nil {
		return nil, scopeError
	}
	rows, queryError := database.queries.ListTraceSpans(ctx, sqlcstore.ListTraceSpansParams{
		ProjectID:     project,
		EnvironmentID: environment,
		TraceID:       traceID,
	})
	if queryError != nil {
		return nil, fmt.Errorf("get trace: %w", queryError)
	}
	spans := make([]query.SpanRecord, 0, len(rows))
	for _, row := range rows {
		spans = append(spans, query.SpanRecord{
			EventID:            row.EventID,
			TraceID:            row.TraceID,
			SpanID:             row.SpanID,
			ParentSpanID:       row.ParentSpanID,
			TraceState:         row.TraceState,
			Name:               row.SpanName,
			Kind:               row.SpanKind,
			StartedAt:          row.StartedAt.Time,
			EndedAt:            row.EndedAt.Time,
			StatusCode:         row.StatusCode,
			StatusMessage:      row.StatusMessage,
			Flags:              row.Flags,
			ServiceName:        row.ServiceName,
			ServiceVersion:     row.ServiceVersion,
			ResourceAttributes: row.ResourceAttributes,
			ScopeName:          row.ScopeName,
			ScopeVersion:       row.ScopeVersion,
			ScopeAttributes:    row.ScopeAttributes,
			Attributes:         row.SpanAttributes,
			Events:             row.SpanEvents,
			Links:              row.SpanLinks,
		})
	}
	return spans, nil
}
