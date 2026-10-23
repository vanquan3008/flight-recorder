package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/incident"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

const incidentDeduplicationWindow = time.Hour

func storeIncidents(ctx context.Context, transaction pgx.Tx, ingestion config.IngestionConfig, events []telemetry.Event) error {
	// Process every candidate event. The occurrence idempotency check inside
	// storeIncident prevents duplicate event IDs while preserving distinct
	// occurrences that share a fingerprint in the same batch.
	for _, event := range events {
		if !incident.Candidate(event) {
			continue
		}
		incidentID, created, incidentError := storeIncident(ctx, transaction, ingestion, event)
		if incidentError != nil {
			return incidentError
		}
		if created {
			if enqueueError := enqueueWebhookDeliveries(ctx, transaction, ingestion.ProjectID, "incident.opened", map[string]any{
				"incident_id": incidentID, "environment_id": ingestion.EnvironmentID, "service": event.ServiceName,
				"title": incident.Title(event), "severity": incident.Severity(event),
			}); enqueueError != nil {
				return enqueueError
			}
		}
	}
	return nil
}

func storeIncident(ctx context.Context, transaction pgx.Tx, ingestion config.IngestionConfig, event telemetry.Event) (string, bool, error) {
	fingerprint := incident.Fingerprint(event)
	if _, lockError := transaction.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", ingestion.ProjectID+":"+ingestion.EnvironmentID+":"+fingerprint); lockError != nil {
		return "", false, fmt.Errorf("lock incident fingerprint: %w", lockError)
	}

	var alreadyRecorded bool
	if queryError := transaction.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM incident_occurrences WHERE project_id = $1 AND event_id = $2)", ingestion.ProjectID, event.ID).Scan(&alreadyRecorded); queryError != nil {
		return "", false, fmt.Errorf("check incident occurrence: %w", queryError)
	}
	if alreadyRecorded {
		return "", false, nil
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
			incident.Title(event), nullableString(event.ErrorType), nullableString(event.ErrorMessage), nullableInt(event.HTTPStatus), incident.Severity(event), event.Timestamp); insertError != nil {
			return "", false, fmt.Errorf("insert incident: %w", insertError)
		}
	} else if queryError != nil {
		return "", false, fmt.Errorf("find incident: %w", queryError)
	}

	occurrenceID := telemetry.DeterministicID(event.Timestamp, "incident-occurrence", incidentID, event.ID)
	if _, insertError := transaction.Exec(ctx, `
		INSERT INTO incident_occurrences (id, project_id, incident_id, event_id, occurred_at)
		VALUES ($1, $2, $3, $4, $5)`, occurrenceID, ingestion.ProjectID, incidentID, event.ID, event.Timestamp); insertError != nil {
		return "", false, fmt.Errorf("insert incident occurrence: %w", insertError)
	}
	if _, insertError := transaction.Exec(ctx, `
		INSERT INTO incident_events (project_id, incident_id, event_id)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`, ingestion.ProjectID, incidentID, event.ID); insertError != nil {
		return "", false, fmt.Errorf("link incident event: %w", insertError)
	}
	if evidenceError := storeEventEvidence(ctx, transaction, ingestion, incidentID, event); evidenceError != nil {
		return "", false, evidenceError
	}
	if newIncident {
		return incidentID, true, nil
	}
	if _, updateError := transaction.Exec(ctx, `
		UPDATE incidents
		SET first_seen_at = LEAST(first_seen_at, $3), last_seen_at = GREATEST(last_seen_at, $3), occurrence_count = occurrence_count + 1,
			trace_id = COALESCE($4, trace_id), request_id = COALESCE($5, request_id),
			error_type = COALESCE($6, error_type), error_message = COALESCE($7, error_message),
			http_status = COALESCE($8, http_status), severity = $9, updated_at = now()
		WHERE project_id = $1 AND id = $2`,
		ingestion.ProjectID, incidentID, event.Timestamp, nullableString(event.TraceID), nullableString(event.RequestID),
		nullableString(event.ErrorType), nullableString(event.ErrorMessage), nullableInt(event.HTTPStatus), incident.Severity(event)); updateError != nil {
		return "", false, fmt.Errorf("update incident: %w", updateError)
	}
	return incidentID, false, nil
}

func storeEventEvidence(ctx context.Context, transaction pgx.Tx, ingestion config.IngestionConfig, incidentID string, event telemetry.Event) error {
	// A detector's summary is derived evidence, not another measured error.
	if event.Signal == "" && event.Type == telemetry.EventCustom && event.ErrorType == "OBSERVABILITY_ANOMALY" && event.Attributes["detector"] == "baseline" {
		return nil
	}
	traceIDs := []string{}
	if event.TraceID != "" {
		traceIDs = append(traceIDs, event.TraceID)
	}
	evidenceID := telemetry.DeterministicID(event.Timestamp, "incident-event-evidence", incidentID, event.ID)
	summary := event.ErrorMessage
	if summary == "" {
		summary = incident.Title(event)
	}
	if _, err := transaction.Exec(ctx, `
		INSERT INTO incident_evidence (id,project_id,environment_id,incident_id,signal_type,entity_type,entity_id,metric_name,direction,observed_value,baseline_value,change_ratio,window_started_at,window_ended_at,trace_ids,summary)
		VALUES ($1,$2,$3,$4,'event','service',$5,'error_event','event',1,0,1,$6,$6,$7,$8)
		ON CONFLICT DO NOTHING`, evidenceID, ingestion.ProjectID, ingestion.EnvironmentID, incidentID, entityID("service", event.ServiceName), event.Timestamp, traceIDs, summary); err != nil {
		return fmt.Errorf("insert event evidence: %w", err)
	}
	return nil
}

func (database *Database) UpdateIncidentStatus(ctx context.Context, projectID, environmentID, incidentID, requestedStatus, actorKeyID string) (incident.Incident, error) {
	status, valid := incident.NormalizeStatus(requestedStatus)
	if !valid || actorKeyID == "" {
		return incident.Incident{}, incident.ErrInvalidTransition
	}
	transaction, beginError := database.pool.Begin(ctx)
	if beginError != nil {
		return incident.Incident{}, fmt.Errorf("begin incident status update: %w", beginError)
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	var previous string
	queryError := transaction.QueryRow(ctx, `
		SELECT status FROM incidents
		WHERE project_id = $1 AND environment_id = $2 AND id = $3
		FOR UPDATE`, projectID, environmentID, incidentID).Scan(&previous)
	if errors.Is(queryError, pgx.ErrNoRows) {
		return incident.Incident{}, incident.ErrNotFound
	}
	if queryError != nil {
		return incident.Incident{}, fmt.Errorf("load incident status: %w", queryError)
	}
	if !incident.CanTransition(previous, status) {
		return incident.Incident{}, incident.ErrInvalidTransition
	}

	changedAt := time.Now().UTC()
	if _, updateError := transaction.Exec(ctx, `
		UPDATE incidents
		SET status = $4, updated_at = $5
		WHERE project_id = $1 AND environment_id = $2 AND id = $3`, projectID, environmentID, incidentID, status, changedAt); updateError != nil {
		return incident.Incident{}, fmt.Errorf("update incident status: %w", updateError)
	}
	changeID := telemetry.DeterministicID(changedAt, "incident-status", projectID, incidentID, previous, status, actorKeyID)
	if _, insertError := transaction.Exec(ctx, `
		INSERT INTO incident_status_changes (id, project_id, incident_id, previous_status, status, actor_key_id, changed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, changeID, projectID, incidentID, previous, status, actorKeyID, changedAt); insertError != nil {
		return incident.Incident{}, fmt.Errorf("record incident status change: %w", insertError)
	}
	if status == "RESOLVED" || status == "OPEN" {
		eventType := "incident.resolved"
		if status == "OPEN" {
			eventType = "incident.opened"
		}
		if enqueueError := enqueueWebhookDeliveries(ctx, transaction, projectID, eventType, map[string]any{
			"incident_id": incidentID, "environment_id": environmentID,
			"previous_status": previous, "status": status, "actor_key_id": actorKeyID,
		}); enqueueError != nil {
			return incident.Incident{}, enqueueError
		}
	}
	if commitError := transaction.Commit(ctx); commitError != nil {
		return incident.Incident{}, fmt.Errorf("commit incident status update: %w", commitError)
	}
	record, incidentError := database.GetIncident(ctx, projectID, environmentID, incidentID)
	if incidentError != nil {
		return incident.Incident{}, incidentError
	}
	return record, nil
}
