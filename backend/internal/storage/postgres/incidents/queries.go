package incidents

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/domain/incident"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

func (store *IncidentStore) ListIncidents(ctx context.Context, projectID string, filter incident.Filter) ([]incident.Incident, error) {
	rows, queryError := store.queries.ListIncidents(ctx, sqlcstore.ListIncidentsParams{
		ProjectID:     projectID,
		EnvironmentID: filter.EnvironmentID,
		Service:       pgvalue.OrNil(filter.Service),
		Severity:      pgvalue.OrNil(filter.Severity),
		Status:        pgvalue.OrNil(filter.Status),
		FromTime:      pgvalue.TimeOrNil(filter.From),
		ToTime:        pgvalue.TimeOrNil(filter.To),
		Before:        pgvalue.TimeOrNil(filter.Before),
		CursorID:      pgvalue.OrNil(filter.CursorID),
		RowLimit:      int32(filter.Limit + 1),
	})
	if queryError != nil {
		return nil, fmt.Errorf("list incidents: %w", queryError)
	}
	incidents := make([]incident.Incident, 0, len(rows))
	for _, row := range rows {
		incidents = append(incidents, incidentRecord(sqlcstore.GetIncidentRow(row)))
	}
	return incidents, nil
}

// GetIncident returns a zero Incident when it is not in the project and environment.
func (store *IncidentStore) GetIncident(ctx context.Context, projectID, environmentID, incidentID string) (incident.Incident, error) {
	row, queryError := store.queries.GetIncident(ctx, sqlcstore.GetIncidentParams{ProjectID: projectID, EnvironmentID: environmentID, ID: incidentID})
	if errors.Is(queryError, pgx.ErrNoRows) {
		return incident.Incident{}, nil
	}
	if queryError != nil {
		return incident.Incident{}, fmt.Errorf("get incident: %w", queryError)
	}
	return incidentRecord(row), nil
}

func (store *IncidentStore) ListIncidentOccurrences(ctx context.Context, projectID, environmentID, incidentID string, limit int, before time.Time, beforeID string) ([]incident.Occurrence, error) {
	rows, queryError := store.queries.ListIncidentOccurrences(ctx, sqlcstore.ListIncidentOccurrencesParams{
		ProjectID: projectID, EnvironmentID: environmentID, IncidentID: incidentID,
		Before: pgvalue.TimeOrNil(before), CursorID: pgvalue.OrNil(beforeID), RowLimit: int32(limit + 1),
	})
	if queryError != nil {
		return nil, fmt.Errorf("list incident occurrences: %w", queryError)
	}
	occurrences := make([]incident.Occurrence, 0, len(rows))
	for _, row := range rows {
		occurrences = append(occurrences, incident.Occurrence{
			ID: row.ID, EventID: row.EventID, OccurredAt: row.OccurredAt, ServiceName: row.ServiceName,
			TraceID: row.TraceID, EventType: row.EventType, Operation: row.Operation,
			ErrorType: row.ErrorType, ErrorMessage: row.ErrorMessage, HTTPStatus: pgvalue.IntPointer(row.HTTPStatus),
		})
	}
	return occurrences, nil
}

func (store *IncidentStore) ListIncidentEvents(ctx context.Context, projectID, environmentID, incidentID string, limit int, after time.Time, afterID string) ([]incident.Event, error) {
	rows, queryError := store.queries.ListIncidentEvents(ctx, sqlcstore.ListIncidentEventsParams{
		ProjectID: projectID, EnvironmentID: environmentID, IncidentID: incidentID,
		After: pgvalue.TimeOrNil(after), CursorID: pgvalue.OrNil(afterID), RowLimit: int32(limit + 1),
	})
	if queryError != nil {
		return nil, fmt.Errorf("list incident events: %w", queryError)
	}
	events := make([]incident.Event, 0, len(rows))
	for _, row := range rows {
		events = append(events, incident.Event{
			EventID: row.ID, OccurredAt: row.OccurredAt, ServiceName: row.ServiceName, TraceID: row.TraceID,
			EventType: row.EventType, Operation: row.Operation, ErrorType: row.ErrorType,
			ErrorMessage: row.ErrorMessage, HTTPStatus: pgvalue.IntPointer(row.HTTPStatus),
		})
	}
	return events, nil
}

func (store *IncidentStore) ListIncidentStatusChanges(ctx context.Context, projectID, environmentID, incidentID string, limit int, before time.Time, beforeID string) ([]incident.StatusChange, error) {
	rows, queryError := store.queries.ListIncidentStatusChanges(ctx, sqlcstore.ListIncidentStatusChangesParams{
		ProjectID: projectID, EnvironmentID: environmentID, IncidentID: incidentID,
		Before: pgvalue.TimeOrNil(before), CursorID: pgvalue.OrNil(beforeID), RowLimit: int32(limit + 1),
	})
	if queryError != nil {
		return nil, fmt.Errorf("list incident status changes: %w", queryError)
	}
	changes := make([]incident.StatusChange, 0, len(rows))
	for _, row := range rows {
		changes = append(changes, incident.StatusChange{
			ID: row.ID, PreviousStatus: row.PreviousStatus, Status: row.Status,
			ActorUserID: derefString(row.ActorUserID), ChangedAt: row.ChangedAt,
		})
	}
	return changes, nil
}

func incidentRecord(row sqlcstore.GetIncidentRow) incident.Incident {
	return incident.Incident{
		ID: row.ID, PrimaryService: row.PrimaryService, TraceID: row.TraceID, RequestID: row.RequestID,
		Fingerprint: row.Fingerprint, Title: row.Title, ErrorType: row.ErrorType, ErrorMessage: row.ErrorMessage,
		HTTPStatus: pgvalue.IntPointer(row.HTTPStatus), Severity: row.Severity, Status: row.Status,
		FirstSeenAt: row.FirstSeenAt, LastSeenAt: row.LastSeenAt, OccurrenceCount: row.OccurrenceCount,
	}
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
