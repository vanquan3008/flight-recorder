package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (database *Database) ListIncidents(ctx context.Context, projectID string, filter IncidentFilter) ([]Incident, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT id, primary_service, COALESCE(trace_id, ''), COALESCE(request_id, ''), fingerprint,
			title, COALESCE(error_type, ''), COALESCE(error_message, ''), http_status, severity, status,
			first_seen_at, last_seen_at, occurrence_count
		FROM incidents
		WHERE project_id = $1 AND environment_id = $2 AND ($3 = '' OR primary_service = $3) AND ($4 = '' OR severity = $4)
			AND ($5 = '' OR status = $5) AND ($6::timestamptz IS NULL OR last_seen_at >= $6)
			AND ($7::timestamptz IS NULL OR last_seen_at <= $7)
			AND ($8::timestamptz IS NULL OR last_seen_at < $8 OR (last_seen_at = $8 AND id < $9::uuid))
		ORDER BY last_seen_at DESC, id DESC LIMIT $10`,
		projectID, filter.EnvironmentID, filter.Service, filter.Severity, filter.Status, nullableTime(filter.From), nullableTime(filter.To),
		nullableTime(filter.Before), nullableString(filter.CursorID), filter.Limit+1)
	if queryError != nil {
		return nil, fmt.Errorf("list incidents: %w", queryError)
	}
	defer rows.Close()
	incidents := make([]Incident, 0, filter.Limit+1)
	for rows.Next() {
		var incident Incident
		if scanError := rows.Scan(&incident.ID, &incident.PrimaryService, &incident.TraceID, &incident.RequestID, &incident.Fingerprint, &incident.Title, &incident.ErrorType, &incident.ErrorMessage, &incident.HTTPStatus, &incident.Severity, &incident.Status, &incident.FirstSeenAt, &incident.LastSeenAt, &incident.OccurrenceCount); scanError != nil {
			return nil, fmt.Errorf("scan incident: %w", scanError)
		}
		incidents = append(incidents, incident)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate incidents: %w", rowsError)
	}
	return incidents, nil
}

func (database *Database) GetIncident(ctx context.Context, projectID, environmentID, incidentID string) (Incident, error) {
	var incident Incident
	queryError := database.pool.QueryRow(ctx, `
		SELECT id, primary_service, COALESCE(trace_id, ''), COALESCE(request_id, ''), fingerprint,
			title, COALESCE(error_type, ''), COALESCE(error_message, ''), http_status, severity, status,
			first_seen_at, last_seen_at, occurrence_count
		FROM incidents WHERE project_id = $1 AND environment_id = $2 AND id = $3`, projectID, environmentID, incidentID).Scan(
		&incident.ID, &incident.PrimaryService, &incident.TraceID, &incident.RequestID, &incident.Fingerprint,
		&incident.Title, &incident.ErrorType, &incident.ErrorMessage, &incident.HTTPStatus, &incident.Severity, &incident.Status,
		&incident.FirstSeenAt, &incident.LastSeenAt, &incident.OccurrenceCount)
	if errors.Is(queryError, pgx.ErrNoRows) {
		return Incident{}, nil
	}
	if queryError != nil {
		return Incident{}, fmt.Errorf("get incident: %w", queryError)
	}
	return incident, nil
}

func (database *Database) ListIncidentOccurrences(ctx context.Context, projectID, environmentID, incidentID string, limit int, before time.Time, beforeID string) ([]IncidentOccurrence, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT occurrence.id, occurrence.event_id, occurrence.occurred_at, event.service_name, COALESCE(event.trace_id, ''),
			event.event_type, COALESCE(event.operation, ''), COALESCE(event.error_type, ''), COALESCE(event.error_message, ''), event.http_status
		FROM incident_occurrences occurrence
		JOIN incidents scoped_incident ON scoped_incident.project_id = occurrence.project_id AND scoped_incident.id = occurrence.incident_id AND scoped_incident.environment_id = $3
		JOIN events event ON event.project_id = occurrence.project_id AND event.id = occurrence.event_id AND event.environment_id = $3
		WHERE occurrence.project_id = $1 AND occurrence.incident_id = $2
			AND ($4::timestamptz IS NULL OR occurrence.occurred_at < $4 OR (occurrence.occurred_at = $4 AND occurrence.id < $5::uuid))
		ORDER BY occurrence.occurred_at DESC, occurrence.id DESC LIMIT $6`,
		projectID, incidentID, environmentID, nullableTime(before), nullableString(beforeID), limit+1)
	if queryError != nil {
		return nil, fmt.Errorf("list incident occurrences: %w", queryError)
	}
	defer rows.Close()
	occurrences := make([]IncidentOccurrence, 0)
	for rows.Next() {
		var occurrence IncidentOccurrence
		if scanError := rows.Scan(&occurrence.ID, &occurrence.EventID, &occurrence.OccurredAt, &occurrence.ServiceName, &occurrence.TraceID, &occurrence.EventType, &occurrence.Operation, &occurrence.ErrorType, &occurrence.ErrorMessage, &occurrence.HTTPStatus); scanError != nil {
			return nil, fmt.Errorf("scan incident occurrence: %w", scanError)
		}
		occurrences = append(occurrences, occurrence)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate incident occurrences: %w", rowsError)
	}
	return occurrences, nil
}

func (database *Database) ListIncidentEvents(ctx context.Context, projectID, environmentID, incidentID string, limit int, after time.Time, afterID string) ([]IncidentEvent, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT event.id, event.occurred_at, event.service_name, COALESCE(event.trace_id, ''), event.event_type,
			COALESCE(event.operation, ''), COALESCE(event.error_type, ''), COALESCE(event.error_message, ''), event.http_status
		FROM incident_events link
		JOIN incidents scoped_incident ON scoped_incident.project_id = link.project_id AND scoped_incident.id = link.incident_id AND scoped_incident.environment_id = $3
		JOIN events event ON event.project_id = link.project_id AND event.id = link.event_id AND event.environment_id = $3
		WHERE link.project_id = $1 AND link.incident_id = $2
			AND ($4::timestamptz IS NULL OR event.occurred_at > $4 OR (event.occurred_at = $4 AND event.id > $5::uuid))
		ORDER BY event.occurred_at ASC, event.id ASC LIMIT $6`,
		projectID, incidentID, environmentID, nullableTime(after), nullableString(afterID), limit+1)
	if queryError != nil {
		return nil, fmt.Errorf("list incident events: %w", queryError)
	}
	defer rows.Close()
	events := make([]IncidentEvent, 0)
	for rows.Next() {
		var event IncidentEvent
		if scanError := rows.Scan(&event.EventID, &event.OccurredAt, &event.ServiceName, &event.TraceID, &event.EventType, &event.Operation, &event.ErrorType, &event.ErrorMessage, &event.HTTPStatus); scanError != nil {
			return nil, fmt.Errorf("scan incident event: %w", scanError)
		}
		events = append(events, event)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate incident events: %w", rowsError)
	}
	return events, nil
}

func (database *Database) ListIncidentStatusChanges(ctx context.Context, projectID, environmentID, incidentID string, limit int, before time.Time, beforeID string) ([]IncidentStatusChange, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT id, previous_status, status, actor_key_id, changed_at FROM incident_status_changes
		WHERE incident_status_changes.project_id = $1 AND incident_status_changes.incident_id = $2
			AND EXISTS (SELECT 1 FROM incidents scoped_incident WHERE scoped_incident.project_id = $1 AND scoped_incident.environment_id = $3 AND scoped_incident.id = incident_status_changes.incident_id)
			AND ($4::timestamptz IS NULL OR changed_at < $4 OR (changed_at = $4 AND id < $5::uuid))
		ORDER BY changed_at DESC, id DESC LIMIT $6`, projectID, incidentID, environmentID, nullableTime(before), nullableString(beforeID), limit+1)
	if queryError != nil {
		return nil, fmt.Errorf("list incident status changes: %w", queryError)
	}
	defer rows.Close()
	changes := make([]IncidentStatusChange, 0, limit+1)
	for rows.Next() {
		var change IncidentStatusChange
		if scanError := rows.Scan(&change.ID, &change.PreviousStatus, &change.Status, &change.ActorKeyID, &change.ChangedAt); scanError != nil {
			return nil, fmt.Errorf("scan incident status change: %w", scanError)
		}
		changes = append(changes, change)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate incident status changes: %w", rowsError)
	}
	return changes, nil
}
