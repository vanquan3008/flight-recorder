package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/query"
)

func (database *Database) ListLogs(ctx context.Context, projectID string, filter query.LogFilter) ([]query.LogRecord, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT event_id, occurred_at, observed_at, COALESCE(trace_id, ''), COALESCE(span_id, ''),
			trace_flags, severity_number, severity_text, body, event_name, service_name,
			COALESCE(service_version, ''), resource_attributes, scope_name, scope_version,
			scope_attributes, log_attributes
		FROM logs
		WHERE project_id = $1 AND environment_id = $2 AND ($3 = '' OR service_name = $3) AND ($4 = '' OR severity_text = $4)
			AND ($5 = '' OR trace_id = $5) AND ($6 = '' OR span_id = $6)
			AND ($7 = '' OR body::text ILIKE '%' || $7 || '%')
			AND ($8::timestamptz IS NULL OR occurred_at >= $8) AND ($9::timestamptz IS NULL OR occurred_at <= $9)
			AND ($10::timestamptz IS NULL OR occurred_at < $10 OR (occurred_at = $10 AND event_id < $11::uuid))
		ORDER BY occurred_at DESC, event_id DESC LIMIT $12`,
		projectID, filter.EnvironmentID, filter.Service, filter.Severity, filter.TraceID, filter.SpanID, filter.Search,
		nullableTime(filter.From), nullableTime(filter.To), nullableTime(filter.Before), nullableString(filter.CursorID), filter.Limit+1)
	if queryError != nil {
		return nil, fmt.Errorf("list logs: %w", queryError)
	}
	defer rows.Close()
	logs := make([]query.LogRecord, 0, filter.Limit+1)
	for rows.Next() {
		var record query.LogRecord
		if scanError := rows.Scan(&record.EventID, &record.OccurredAt, &record.ObservedAt, &record.TraceID, &record.SpanID, &record.TraceFlags, &record.SeverityNumber, &record.SeverityText, &record.Body, &record.EventName, &record.ServiceName, &record.ServiceVersion, &record.ResourceAttributes, &record.ScopeName, &record.ScopeVersion, &record.ScopeAttributes, &record.Attributes); scanError != nil {
			return nil, fmt.Errorf("scan log: %w", scanError)
		}
		logs = append(logs, record)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate logs: %w", rowsError)
	}
	return logs, nil
}

func (database *Database) GetLog(ctx context.Context, projectID, environmentID, eventID string) (query.LogRecord, error) {
	var record query.LogRecord
	queryError := database.pool.QueryRow(ctx, `
		SELECT event_id, occurred_at, observed_at, COALESCE(trace_id, ''), COALESCE(span_id, ''),
			trace_flags, severity_number, severity_text, body, event_name, service_name,
			COALESCE(service_version, ''), resource_attributes, scope_name, scope_version,
			scope_attributes, log_attributes
		FROM logs WHERE project_id = $1 AND environment_id = $2 AND event_id = $3::uuid`, projectID, environmentID, eventID).Scan(
		&record.EventID, &record.OccurredAt, &record.ObservedAt, &record.TraceID, &record.SpanID,
		&record.TraceFlags, &record.SeverityNumber, &record.SeverityText, &record.Body, &record.EventName, &record.ServiceName,
		&record.ServiceVersion, &record.ResourceAttributes, &record.ScopeName, &record.ScopeVersion, &record.ScopeAttributes, &record.Attributes)
	if errors.Is(queryError, pgx.ErrNoRows) {
		return query.LogRecord{}, nil
	}
	if queryError != nil {
		return query.LogRecord{}, fmt.Errorf("get log: %w", queryError)
	}
	return record, nil
}
