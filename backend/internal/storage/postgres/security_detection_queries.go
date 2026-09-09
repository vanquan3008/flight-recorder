package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/security"
)

const securityEventSelectColumns = `
	id, raw_event_id, COALESCE(source_id::text, ''), occurred_at, format, event_kind,
	COALESCE(category, ''), COALESCE(action, ''), COALESCE(outcome, ''), COALESCE(severity, ''),
	COALESCE(observer_vendor, ''), COALESCE(observer_product, ''), COALESCE(observer_hostname, ''),
	COALESCE(host_name, ''), COALESCE(user_name, ''),
	COALESCE(source_ip::text, ''), COALESCE(source_port, 0),
	COALESCE(destination_ip::text, ''), COALESCE(destination_port, 0),
	COALESCE(network_transport, ''), COALESCE(network_protocol, ''),
	COALESCE(rule_identifier, ''), COALESCE(rule_name, ''), COALESCE(message, ''), attributes`

func (database *Database) SecurityDetectionHealth(ctx context.Context, ingestion config.IngestionConfig) (time.Duration, time.Duration, error) {
	var latest, checkpoint, checkpointUpdated time.Time
	if err := database.pool.QueryRow(ctx,
		`SELECT COALESCE(max(ingested_at), 'epoch'::timestamptz) FROM security_events WHERE project_id=$1 AND environment_id=$2`,
		ingestion.ProjectID, ingestion.EnvironmentID,
	).Scan(&latest); err != nil {
		return 0, 0, fmt.Errorf("load latest security event: %w", err)
	}
	err := database.pool.QueryRow(ctx,
		`SELECT last_ingested_at, updated_at FROM security_detection_checkpoints WHERE project_id=$1 AND environment_id=$2`,
		ingestion.ProjectID, ingestion.EnvironmentID,
	).Scan(&checkpoint, &checkpointUpdated)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("load security detection checkpoint: %w", err)
	}
	lag := latest.Sub(checkpoint)
	if latest.Equal(time.Unix(0, 0).UTC()) || lag < 0 {
		lag = 0
	}
	age := time.Since(checkpointUpdated)
	if age < 0 {
		age = 0
	}
	return lag, age, nil
}

func securityDetectionExists(ctx context.Context, tx pgx.Tx, ingestion config.IngestionConfig, rule security.Rule, event security.Event) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM security_detections WHERE project_id=$1 AND rule_id=$2 AND security_event_id=$3)`,
		ingestion.ProjectID, rule.ID, event.ID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check security detection idempotency: %w", err)
	}
	return exists, nil
}

func securityCheckpoint(ctx context.Context, tx pgx.Tx, ingestion config.IngestionConfig) (time.Time, string, error) {
	var at time.Time
	var id string
	err := tx.QueryRow(ctx,
		`SELECT last_ingested_at, last_event_id FROM security_detection_checkpoints
		 WHERE project_id=$1 AND environment_id=$2 FOR UPDATE`,
		ingestion.ProjectID, ingestion.EnvironmentID,
	).Scan(&at, &id)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, "", nil
	}
	if err != nil {
		return time.Time{}, "", fmt.Errorf("load security checkpoint: %w", err)
	}
	return at, id, nil
}

func securityEventsAfter(ctx context.Context, tx pgx.Tx, ingestion config.IngestionConfig, after time.Time, afterID string) ([]security.Event, []time.Time, error) {
	query := `SELECT ` + securityEventSelectColumns + `, ingested_at FROM security_events WHERE project_id=$1 AND environment_id=$2`
	arguments := []any{ingestion.ProjectID, ingestion.EnvironmentID}
	if !after.IsZero() {
		query += " AND (ingested_at > $3 OR (ingested_at = $3 AND id > $4::uuid))"
		arguments = append(arguments, after, afterID)
	}
	query += fmt.Sprintf(" ORDER BY ingested_at, id LIMIT %d", securityDetectionBatchSize)
	rows, err := tx.Query(ctx, query, arguments...)
	if err != nil {
		return nil, nil, fmt.Errorf("list new security events: %w", err)
	}
	defer rows.Close()
	events := make([]security.Event, 0, securityDetectionBatchSize)
	ingested := make([]time.Time, 0, securityDetectionBatchSize)
	for rows.Next() {
		record, scanError := scanSecurityEventWithIngested(rows)
		if scanError != nil {
			return nil, nil, scanError
		}
		events = append(events, record.event)
		ingested = append(ingested, record.ingestedAt)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate new security events: %w", err)
	}
	return events, ingested, nil
}

type securityEventWithIngested struct {
	event      security.Event
	ingestedAt time.Time
}

func scanSecurityEventWithIngested(row securityEventScanner) (securityEventWithIngested, error) {
	var record securityEventWithIngested
	var attributes []byte
	if err := row.Scan(
		&record.event.ID, &record.event.RawEventID, &record.event.SourceID,
		&record.event.OccurredAt, &record.event.Format, &record.event.Kind,
		&record.event.Category, &record.event.Action, &record.event.Outcome, &record.event.Severity,
		&record.event.ObserverVendor, &record.event.ObserverProduct, &record.event.ObserverHostname,
		&record.event.HostName, &record.event.UserName,
		&record.event.SourceIP, &record.event.SourcePort,
		&record.event.DestinationIP, &record.event.DestinationPort,
		&record.event.NetworkTransport, &record.event.NetworkProtocol,
		&record.event.RuleIdentifier, &record.event.RuleName, &record.event.Message,
		&attributes, &record.ingestedAt,
	); err != nil {
		return securityEventWithIngested{}, err
	}
	if err := json.Unmarshal(attributes, &record.event.Attributes); err != nil {
		return securityEventWithIngested{}, err
	}
	return record, nil
}

func activeSecurityRules(ctx context.Context, tx pgx.Tx, projectID string) ([]security.Rule, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, name, COALESCE(description, ''), version, enabled, severity,
		       match_field, match_operator, match_value, threshold_count,
		       COALESCE(group_by_field, ''), window_seconds, created_at, updated_at
		FROM security_detection_rules
		WHERE project_id=$1 AND enabled=true
		ORDER BY id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list active security rules: %w", err)
	}
	defer rows.Close()
	rules := make([]security.Rule, 0)
	for rows.Next() {
		rule, scanError := scanSecurityRule(rows)
		if scanError != nil {
			return nil, scanError
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active security rules: %w", err)
	}
	return rules, nil
}

func countMatchingSecurityEvents(ctx context.Context, tx pgx.Tx, ingestion config.IngestionConfig, rule security.Rule, current security.Event, groupKey string) (int, error) {
	fetchLimit := rule.Threshold * 10
	if fetchLimit < 100 {
		fetchLimit = 100
	}
	rows, err := tx.Query(ctx,
		`SELECT `+securityEventSelectColumns+`
		 FROM security_events
		 WHERE project_id=$1 AND environment_id=$2
		   AND occurred_at >= $3 AND occurred_at <= $4
		 ORDER BY occurred_at DESC LIMIT $5`,
		ingestion.ProjectID, ingestion.EnvironmentID,
		current.OccurredAt.Add(-rule.Window), current.OccurredAt, fetchLimit,
	)
	if err != nil {
		return 0, fmt.Errorf("count matching security events: %w", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		event, scanError := scanSecurityEvent(rows)
		if scanError != nil {
			return 0, scanError
		}
		if security.Matches(rule, event) && event.Field(rule.GroupBy) == groupKey {
			count++
			if count >= rule.Threshold {
				return count, nil
			}
		}
	}
	return count, rows.Err()
}
