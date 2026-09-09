package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/security"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func storeSecurityIncident(ctx context.Context, tx pgx.Tx, ingestion config.IngestionConfig, rule security.Rule, event security.Event, groupKey string) (string, error) {
	fingerprint := "security:" + rule.ID + ":" + groupKey
	lockKey := ingestion.ProjectID + ":" + ingestion.EnvironmentID + ":" + fingerprint
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", lockKey); err != nil {
		return "", err
	}

	var incidentID string
	created := false
	err := tx.QueryRow(ctx, `
		SELECT id FROM incidents
		WHERE project_id=$1 AND environment_id=$2 AND fingerprint=$3 AND status IN ('OPEN','INVESTIGATING')
		ORDER BY last_seen_at DESC LIMIT 1`,
		ingestion.ProjectID, ingestion.EnvironmentID, fingerprint,
	).Scan(&incidentID)
	if errors.Is(err, pgx.ErrNoRows) {
		incidentID = telemetry.DeterministicID(event.OccurredAt, "security-incident", ingestion.ProjectID, rule.ID, groupKey, event.ID)
		_, err = tx.Exec(ctx, `
			INSERT INTO incidents (id,project_id,environment_id,primary_service,fingerprint,title,error_type,error_message,severity,status,first_seen_at,last_seen_at)
			VALUES ($1,$2,$3,'security',$4,$5,$6,$7,$8,'OPEN',$9,$9)`,
			incidentID, ingestion.ProjectID, ingestion.EnvironmentID,
			fingerprint, rule.Name, nullableString(event.RuleIdentifier),
			nullableString(event.Message), rule.Severity, event.OccurredAt,
		)
		if err != nil {
			return "", fmt.Errorf("create security incident: %w", err)
		}
		created = true
	} else if err != nil {
		return "", fmt.Errorf("find security incident: %w", err)
	}

	occurrenceID := telemetry.DeterministicID(event.OccurredAt, "security-incident-occurrence", incidentID, event.RawEventID)
	result, err := tx.Exec(ctx, `
		INSERT INTO incident_occurrences (id,project_id,incident_id,event_id,occurred_at)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (incident_id,event_id) DO NOTHING`,
		occurrenceID, ingestion.ProjectID, incidentID, event.RawEventID, event.OccurredAt,
	)
	if err != nil {
		return "", err
	}
	if !created && result.RowsAffected() == 1 {
		_, err = tx.Exec(ctx, `
			UPDATE incidents
			SET last_seen_at=GREATEST(last_seen_at,$3), occurrence_count=occurrence_count+1,
			    severity=$4, error_message=COALESCE($5,error_message), updated_at=now()
			WHERE project_id=$1 AND id=$2`,
			ingestion.ProjectID, incidentID, event.OccurredAt,
			rule.Severity, nullableString(event.Message),
		)
		if err != nil {
			return "", err
		}
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO incident_events (project_id,incident_id,event_id) VALUES ($1,$2,$3)
		ON CONFLICT DO NOTHING`,
		ingestion.ProjectID, incidentID, event.RawEventID,
	)
	if err != nil {
		return "", err
	}
	return incidentID, nil
}
