package explorer

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

func (store *QueryStore) ListLogs(ctx context.Context, projectID string, filter query.LogFilter) ([]query.LogRecord, error) {
	rows, queryError := store.queries.ListLogs(ctx, sqlcstore.ListLogsParams{
		ProjectID:     projectID,
		EnvironmentID: filter.EnvironmentID,
		Service:       pgvalue.OrNil(filter.Service),
		Severity:      pgvalue.OrNil(filter.Severity),
		TraceID:       pgvalue.OrNil(filter.TraceID),
		SpanID:        pgvalue.OrNil(filter.SpanID),
		Search:        pgvalue.OrNil(filter.Search),
		FromTime:      pgvalue.TimeOrNil(filter.From),
		ToTime:        pgvalue.TimeOrNil(filter.To),
		Before:        pgvalue.TimeOrNil(filter.Before),
		CursorID:      pgvalue.OrNil(filter.CursorID),
		RowLimit:      int32(filter.Limit + 1),
	})
	if queryError != nil {
		return nil, fmt.Errorf("list logs: %w", queryError)
	}
	logs := make([]query.LogRecord, 0, len(rows))
	for _, row := range rows {
		logs = append(logs, logRecord(sqlcstore.GetLogRow(row)))
	}
	return logs, nil
}

// GetLog returns a zero LogRecord when the log is not in the project and environment.
func (store *QueryStore) GetLog(ctx context.Context, projectID, environmentID, eventID string) (query.LogRecord, error) {
	row, queryError := store.queries.GetLog(ctx, sqlcstore.GetLogParams{ProjectID: projectID, EnvironmentID: environmentID, EventID: eventID})
	if errors.Is(queryError, pgx.ErrNoRows) {
		return query.LogRecord{}, nil
	}
	if queryError != nil {
		return query.LogRecord{}, fmt.Errorf("get log: %w", queryError)
	}
	return logRecord(row), nil
}

func logRecord(row sqlcstore.GetLogRow) query.LogRecord {
	return query.LogRecord{
		EventID: row.EventID, OccurredAt: row.OccurredAt, ObservedAt: row.ObservedAt,
		TraceID: row.TraceID, SpanID: row.SpanID, TraceFlags: row.TraceFlags,
		SeverityNumber: row.SeverityNumber, SeverityText: row.SeverityText, Body: row.Body,
		EventName: row.EventName, ServiceName: row.ServiceName, ServiceVersion: row.ServiceVersion,
		ResourceAttributes: row.ResourceAttributes, ScopeName: row.ScopeName, ScopeVersion: row.ScopeVersion,
		ScopeAttributes: row.ScopeAttributes, Attributes: row.LogAttributes,
	}
}
