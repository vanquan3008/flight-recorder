package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/telemetry"
)

const incidentDeduplicationWindow = time.Hour

var (
	ErrIncidentNotFound          = errors.New("incident not found")
	ErrInvalidIncidentTransition = errors.New("invalid incident status transition")
)

func NormalizeIncidentStatus(value string) (string, bool) {
	status := strings.ToUpper(strings.TrimSpace(value))
	switch status {
	case "OPEN", "INVESTIGATING", "RESOLVED", "IGNORED":
		return status, true
	default:
		return "", false
	}
}

func canTransitionIncidentStatus(previous, next string) bool {
	switch previous {
	case "OPEN":
		return next == "INVESTIGATING" || next == "RESOLVED" || next == "IGNORED"
	case "INVESTIGATING":
		return next == "RESOLVED" || next == "IGNORED"
	case "RESOLVED", "IGNORED":
		return next == "OPEN"
	default:
		return false
	}
}

func storeIncidents(ctx context.Context, transaction pgx.Tx, ingestion config.IngestionConfig, events []telemetry.Event) error {
	for _, event := range events {
		if !incidentCandidate(event) {
			continue
		}
		if incidentError := storeIncident(ctx, transaction, ingestion, event); incidentError != nil {
			return incidentError
		}
	}
	return nil
}

func incidentCandidate(event telemetry.Event) bool {
	if event.HTTPStatus >= 500 || event.Type == telemetry.EventException {
		return true
	}
	severity := strings.ToUpper(event.Severity)
	return strings.Contains(severity, "ERROR") || strings.Contains(severity, "FATAL")
}

func storeIncident(ctx context.Context, transaction pgx.Tx, ingestion config.IngestionConfig, event telemetry.Event) error {
	fingerprint := incidentFingerprint(event)
	if _, lockError := transaction.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", ingestion.ProjectID+":"+ingestion.EnvironmentID+":"+fingerprint); lockError != nil {
		return fmt.Errorf("lock incident fingerprint: %w", lockError)
	}

	var alreadyRecorded bool
	if queryError := transaction.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM incident_occurrences WHERE project_id = $1 AND event_id = $2)", ingestion.ProjectID, event.ID).Scan(&alreadyRecorded); queryError != nil {
		return fmt.Errorf("check incident occurrence: %w", queryError)
	}
	if alreadyRecorded {
		return nil
	}

	cutoff := event.Timestamp.Add(-incidentDeduplicationWindow)
	ceiling := event.Timestamp.Add(incidentDeduplicationWindow)
	var incidentID string
	newIncident := false
	queryError := transaction.QueryRow(ctx, `
		SELECT id FROM incidents
		WHERE project_id = $1 AND environment_id = $2 AND fingerprint = $3
			AND last_seen_at BETWEEN $4 AND $5
		ORDER BY last_seen_at DESC, id DESC
		LIMIT 1`, ingestion.ProjectID, ingestion.EnvironmentID, fingerprint, cutoff, ceiling).Scan(&incidentID)
	if errors.Is(queryError, pgx.ErrNoRows) {
		newIncident = true
		incidentID = telemetry.DeterministicID(event.Timestamp, "incident", ingestion.ProjectID, ingestion.EnvironmentID, fingerprint, event.ID)
		if _, insertError := transaction.Exec(ctx, `
			INSERT INTO incidents (
				id, project_id, environment_id, primary_service, trace_id, request_id, fingerprint,
				title, error_type, error_message, http_status, severity, status, first_seen_at, last_seen_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7,
				$8, $9, $10, $11, $12, 'OPEN', $13, $13
			)`,
			incidentID, ingestion.ProjectID, ingestion.EnvironmentID, event.ServiceName, nullableString(event.TraceID), nullableString(event.RequestID), fingerprint,
			incidentTitle(event), nullableString(event.ErrorType), nullableString(event.ErrorMessage), nullableInt(event.HTTPStatus), incidentSeverity(event), event.Timestamp); insertError != nil {
			return fmt.Errorf("insert incident: %w", insertError)
		}
	} else if queryError != nil {
		return fmt.Errorf("find incident: %w", queryError)
	}

	occurrenceID := telemetry.DeterministicID(event.Timestamp, "incident-occurrence", incidentID, event.ID)
	if _, insertError := transaction.Exec(ctx, `
		INSERT INTO incident_occurrences (id, project_id, incident_id, event_id, occurred_at)
		VALUES ($1, $2, $3, $4, $5)`, occurrenceID, ingestion.ProjectID, incidentID, event.ID, event.Timestamp); insertError != nil {
		return fmt.Errorf("insert incident occurrence: %w", insertError)
	}
	if _, insertError := transaction.Exec(ctx, `
		INSERT INTO incident_events (project_id, incident_id, event_id)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`, ingestion.ProjectID, incidentID, event.ID); insertError != nil {
		return fmt.Errorf("link incident event: %w", insertError)
	}
	if newIncident {
		return nil
	}
	if _, updateError := transaction.Exec(ctx, `
		UPDATE incidents
		SET last_seen_at = GREATEST(last_seen_at, $3), occurrence_count = occurrence_count + 1,
			trace_id = COALESCE($4, trace_id), request_id = COALESCE($5, request_id),
			error_type = COALESCE($6, error_type), error_message = COALESCE($7, error_message),
			http_status = COALESCE($8, http_status), severity = $9, updated_at = now()
		WHERE project_id = $1 AND id = $2`,
		ingestion.ProjectID, incidentID, event.Timestamp, nullableString(event.TraceID), nullableString(event.RequestID),
		nullableString(event.ErrorType), nullableString(event.ErrorMessage), nullableInt(event.HTTPStatus), incidentSeverity(event)); updateError != nil {
		return fmt.Errorf("update incident: %w", updateError)
	}
	return nil
}

func (database *Database) UpdateIncidentStatus(ctx context.Context, projectID, environmentID, incidentID, requestedStatus, actorKeyID string) (Incident, error) {
	status, valid := NormalizeIncidentStatus(requestedStatus)
	if !valid || actorKeyID == "" {
		return Incident{}, ErrInvalidIncidentTransition
	}
	transaction, beginError := database.pool.Begin(ctx)
	if beginError != nil {
		return Incident{}, fmt.Errorf("begin incident status update: %w", beginError)
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	var previous string
	queryError := transaction.QueryRow(ctx, `
		SELECT status FROM incidents
		WHERE project_id = $1 AND environment_id = $2 AND id = $3
		FOR UPDATE`, projectID, environmentID, incidentID).Scan(&previous)
	if errors.Is(queryError, pgx.ErrNoRows) {
		return Incident{}, ErrIncidentNotFound
	}
	if queryError != nil {
		return Incident{}, fmt.Errorf("load incident status: %w", queryError)
	}
	if !canTransitionIncidentStatus(previous, status) {
		return Incident{}, ErrInvalidIncidentTransition
	}

	changedAt := time.Now().UTC()
	if _, updateError := transaction.Exec(ctx, `
		UPDATE incidents
		SET status = $4, updated_at = $5
		WHERE project_id = $1 AND environment_id = $2 AND id = $3`, projectID, environmentID, incidentID, status, changedAt); updateError != nil {
		return Incident{}, fmt.Errorf("update incident status: %w", updateError)
	}
	changeID := telemetry.DeterministicID(changedAt, "incident-status", projectID, incidentID, previous, status, actorKeyID)
	if _, insertError := transaction.Exec(ctx, `
		INSERT INTO incident_status_changes (id, project_id, incident_id, previous_status, status, actor_key_id, changed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, changeID, projectID, incidentID, previous, status, actorKeyID, changedAt); insertError != nil {
		return Incident{}, fmt.Errorf("record incident status change: %w", insertError)
	}
	if commitError := transaction.Commit(ctx); commitError != nil {
		return Incident{}, fmt.Errorf("commit incident status update: %w", commitError)
	}
	incident, incidentError := database.GetIncident(ctx, projectID, environmentID, incidentID)
	if incidentError != nil {
		return Incident{}, incidentError
	}
	return incident, nil
}

func incidentFingerprint(event telemetry.Event) string {
	identity := strings.Join([]string{event.ServiceName, event.Operation, event.ErrorType, event.ErrorMessage, fmt.Sprintf("%d", event.HTTPStatus)}, "\x00")
	digest := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(digest[:])
}

func incidentTitle(event telemetry.Event) string {
	if event.Operation != "" {
		return event.ServiceName + " " + event.Operation + " failed"
	}
	if event.ErrorType != "" {
		return event.ServiceName + " " + event.ErrorType
	}
	return event.ServiceName + " failure"
}

func incidentSeverity(event telemetry.Event) string {
	if strings.Contains(strings.ToUpper(event.Severity), "FATAL") {
		return "CRITICAL"
	}
	return "ERROR"
}
