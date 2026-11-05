package clickhouse

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
)

// StoreLogRecords preserves IDs so backfill can be interrupted and rerun.
func (store *Store) StoreLogRecords(ctx context.Context, projectID, environmentID string, records []query.LogRecord) error {
	rows := make([]storedLog, 0, len(records))
	for _, record := range records {
		row := storedLog{
			ProjectID: projectID, EnvironmentID: environmentID, EventID: record.EventID,
			OccurredAt: record.OccurredAt.UTC(), ObservedAt: record.ObservedAt,
			TraceID: record.TraceID, SpanID: record.SpanID, TraceFlags: record.TraceFlags,
			SeverityNumber: record.SeverityNumber, SeverityText: record.SeverityText,
			Body: rawJSON(record.Body, "null"), EventName: record.EventName,
			ServiceName: record.ServiceName, ServiceVersion: record.ServiceVersion,
			ResourceAttributes: rawJSON(record.ResourceAttributes, "{}"),
			ScopeName:          record.ScopeName, ScopeVersion: record.ScopeVersion,
			ScopeAttributes: rawJSON(record.ScopeAttributes, "{}"), Attributes: rawJSON(record.Attributes, "{}"),
		}
		if _, err := row.record(); err != nil {
			return fmt.Errorf("invalid backfill log JSON: %w", telemetry.ErrInvalidEvent)
		}
		rows = append(rows, row)
	}
	return store.insertLogs(ctx, projectID, environmentID, rows)
}

func rawJSON(value json.RawMessage, fallback string) string {
	if len(value) == 0 {
		return fallback
	}
	return string(value)
}
