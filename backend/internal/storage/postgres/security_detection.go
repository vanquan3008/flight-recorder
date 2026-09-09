package postgres

import (
	"context"
	"fmt"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/security"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

const securityDetectionBatchSize = 100

// EvaluateSecurityRules advances a durable checkpoint only after all matching
// detections are committed. It is intentionally separate from StoreEvents so a
// slow rule or a faulty rule does not block durable ingestion.
func (database *Database) EvaluateSecurityRules(ctx context.Context, ingestion config.IngestionConfig) (int, error) {
	transaction, beginError := database.pool.Begin(ctx)
	if beginError != nil {
		return 0, fmt.Errorf("begin security detection: %w", beginError)
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	// Serialize this project/environment even before its first checkpoint row
	// exists. Without this lock, two replicas can both process the first batch
	// and inflate the linked incident occurrence count.
	lockKey := ingestion.ProjectID + ":" + ingestion.EnvironmentID + ":security-detection"
	if _, lockError := transaction.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", lockKey); lockError != nil {
		return 0, fmt.Errorf("lock security detection: %w", lockError)
	}

	lastIngestedAt, lastEventID, checkpointError := securityCheckpoint(ctx, transaction, ingestion)
	if checkpointError != nil {
		return 0, checkpointError
	}

	events, ingestTimes, eventsError := securityEventsAfter(ctx, transaction, ingestion, lastIngestedAt, lastEventID)
	if eventsError != nil {
		return 0, eventsError
	}
	if len(events) == 0 {
		return 0, transaction.Commit(ctx)
	}

	rules, rulesError := activeSecurityRules(ctx, transaction, ingestion.ProjectID)
	if rulesError != nil {
		return 0, rulesError
	}

	detections := 0
	for index, event := range events {
		for _, rule := range rules {
			if !security.Matches(rule, event) {
				continue
			}
			alreadyDetected, detectionCheckError := securityDetectionExists(ctx, transaction, ingestion, rule, event)
			if detectionCheckError != nil {
				return detections, detectionCheckError
			}
			if alreadyDetected {
				continue
			}
			groupKey := event.Field(rule.GroupBy)
			if rule.Threshold > 1 {
				count, countError := countMatchingSecurityEvents(ctx, transaction, ingestion, rule, event, groupKey)
				if countError != nil {
					return detections, countError
				}
				if count < rule.Threshold {
					continue
				}
			}
			incidentID, incidentError := storeSecurityIncident(ctx, transaction, ingestion, rule, event, groupKey)
			if incidentError != nil {
				return detections, incidentError
			}
			windowStart := event.OccurredAt.Add(-rule.Window)
			detectionID := telemetry.DeterministicID(event.OccurredAt, "security-detection", rule.ID, fmt.Sprintf("%d", rule.Version), event.ID)
			result, insertError := transaction.Exec(ctx, `
				INSERT INTO security_detections (
					id, project_id, environment_id, rule_id, rule_version,
					security_event_id, incident_id, group_key, window_started_at, window_ended_at, severity
				) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
				ON CONFLICT (project_id, rule_id, security_event_id) DO NOTHING`,
				detectionID, ingestion.ProjectID, ingestion.EnvironmentID, rule.ID, rule.Version,
				event.ID, nullableString(incidentID), groupKey, windowStart, event.OccurredAt, rule.Severity,
			)
			if insertError != nil {
				return detections, fmt.Errorf("insert security detection: %w", insertError)
			}
			if result.RowsAffected() == 1 {
				detections++
				if enqueueError := enqueueWebhookDeliveries(ctx, transaction, ingestion.ProjectID, "detection.created", map[string]any{
					"detection_id": detectionID, "rule_id": rule.ID, "rule_version": rule.Version,
					"security_event_id": event.ID, "incident_id": incidentID,
					"environment_id": ingestion.EnvironmentID, "severity": rule.Severity,
				}); enqueueError != nil {
					return detections, enqueueError
				}
			}
		}
		lastIngestedAt, lastEventID = ingestTimes[index], event.ID
	}

	if _, updateError := transaction.Exec(ctx, `
		INSERT INTO security_detection_checkpoints (project_id, environment_id, last_ingested_at, last_event_id)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (project_id, environment_id)
		DO UPDATE SET last_ingested_at=EXCLUDED.last_ingested_at, last_event_id=EXCLUDED.last_event_id, updated_at=now()`,
		ingestion.ProjectID, ingestion.EnvironmentID, lastIngestedAt, lastEventID,
	); updateError != nil {
		return detections, fmt.Errorf("update security checkpoint: %w", updateError)
	}

	if commitError := transaction.Commit(ctx); commitError != nil {
		return detections, fmt.Errorf("commit security detection: %w", commitError)
	}
	return detections, nil
}
