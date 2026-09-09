package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/security"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func storeSecurityEvents(ctx context.Context, transaction pgx.Tx, ingestion config.IngestionConfig, events []telemetry.Event) error {
	for _, rawEvent := range events {
		event := rawEvent.Security
		if event == nil {
			continue
		}
		if event.RawEventID != rawEvent.ID {
			return errors.New("security event raw event identity mismatch")
		}
		if validationError := event.Validate(); validationError != nil {
			return fmt.Errorf("validate security event: %w", validationError)
		}
		attributes, marshalError := json.Marshal(event.Attributes)
		if marshalError != nil {
			return fmt.Errorf("encode security event attributes: %w", marshalError)
		}
		if event.SourceID != "" {
			sourceName := event.SourceName
			if sourceName == "" {
				sourceName = event.SourceID
			}
			if _, sourceError := transaction.Exec(ctx, `INSERT INTO security_sources (id, project_id, environment_id, name, source_type, format, last_seen_at) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (project_id, environment_id, name) DO UPDATE SET format=EXCLUDED.format, updated_at=now()`, event.SourceID, ingestion.ProjectID, ingestion.EnvironmentID, sourceName, securitySourceType(rawEvent), event.Format, event.OccurredAt); sourceError != nil {
				return fmt.Errorf("upsert security source: %w", sourceError)
			}
		}
		result, insertError := transaction.Exec(ctx, `
			INSERT INTO security_events (
				id, project_id, environment_id, raw_event_id, source_id, occurred_at, format, event_kind, category, action, outcome, severity,
				observer_vendor, observer_product, observer_hostname, host_name, user_name, source_ip, source_port, destination_ip,
				destination_port, network_transport, network_protocol, rule_identifier, rule_name, message, attributes
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
				$13, $14, $15, $16, $17, $18::inet, $19, $20::inet,
				$21, $22, $23, $24, $25, $26, $27
			) ON CONFLICT (project_id, raw_event_id) DO NOTHING`,
			event.ID, ingestion.ProjectID, ingestion.EnvironmentID, event.RawEventID, nullableString(event.SourceID), event.OccurredAt, event.Format, event.Kind, nullableString(event.Category), nullableString(event.Action), nullableString(event.Outcome), nullableString(event.Severity),
			nullableString(event.ObserverVendor), nullableString(event.ObserverProduct), nullableString(event.ObserverHostname), nullableString(event.HostName), nullableString(event.UserName), nullableString(event.SourceIP), nullableInt(event.SourcePort), nullableString(event.DestinationIP),
			nullableInt(event.DestinationPort), nullableString(event.NetworkTransport), nullableString(event.NetworkProtocol), nullableString(event.RuleIdentifier), nullableString(event.RuleName), nullableString(event.Message), attributes,
		)
		if insertError != nil {
			return fmt.Errorf("insert security event: %w", insertError)
		}
		if result.RowsAffected() == 1 && event.SourceID != "" {
			if _, sourceError := transaction.Exec(ctx, `UPDATE security_sources SET last_seen_at=GREATEST(last_seen_at, $4), accepted_count=accepted_count+1, updated_at=now() WHERE project_id=$1 AND environment_id=$2 AND id=$3`, ingestion.ProjectID, ingestion.EnvironmentID, event.SourceID, event.OccurredAt); sourceError != nil {
				return fmt.Errorf("update security source: %w", sourceError)
			}
		}
	}
	return nil
}

func (database *Database) ListSecurityEvents(ctx context.Context, projectID string, filter security.Filter) ([]security.Event, error) {
	rows, queryError := database.pool.Query(ctx, `
		SELECT id, raw_event_id, COALESCE(source_id::text, ''), occurred_at, format, event_kind, COALESCE(category, ''), COALESCE(action, ''), COALESCE(outcome, ''), COALESCE(severity, ''),
			COALESCE(observer_vendor, ''), COALESCE(observer_product, ''), COALESCE(observer_hostname, ''), COALESCE(host_name, ''), COALESCE(user_name, ''),
			COALESCE(source_ip::text, ''), COALESCE(source_port, 0), COALESCE(destination_ip::text, ''), COALESCE(destination_port, 0),
			COALESCE(network_transport, ''), COALESCE(network_protocol, ''), COALESCE(rule_identifier, ''), COALESCE(rule_name, ''), COALESCE(message, ''), attributes
		FROM security_events
		WHERE project_id = $1 AND environment_id = $2 AND ($3 = '' OR event_kind = $3) AND ($4 = '' OR severity = $4)
			AND ($5 = '' OR source_ip = NULLIF($5, '')::inet) AND ($6 = '' OR destination_ip = NULLIF($6, '')::inet) AND ($7 = '' OR outcome = $7)
			AND ($8::timestamptz IS NULL OR occurred_at >= $8) AND ($9::timestamptz IS NULL OR occurred_at <= $9)
			AND ($10::timestamptz IS NULL OR occurred_at < $10 OR (occurred_at = $10 AND id < $11::uuid))
		ORDER BY occurred_at DESC, id DESC LIMIT $12`,
		projectID, filter.EnvironmentID, filter.Kind, filter.Severity, filter.SourceIP, filter.DestinationIP, filter.Outcome,
		nullableTime(filter.From), nullableTime(filter.To), nullableTime(filter.Before), nullableString(filter.CursorID), filter.Limit+1)
	if queryError != nil {
		return nil, fmt.Errorf("list security events: %w", queryError)
	}
	defer rows.Close()
	events := make([]security.Event, 0, filter.Limit+1)
	for rows.Next() {
		event, scanError := scanSecurityEvent(rows)
		if scanError != nil {
			return nil, scanError
		}
		events = append(events, event)
	}
	if rowsError := rows.Err(); rowsError != nil {
		return nil, fmt.Errorf("iterate security events: %w", rowsError)
	}
	return events, nil
}

func (database *Database) GetSecurityEvent(ctx context.Context, projectID, environmentID, eventID string) (security.Event, error) {
	row := database.pool.QueryRow(ctx, `
		SELECT id, raw_event_id, COALESCE(source_id::text, ''), occurred_at, format, event_kind, COALESCE(category, ''), COALESCE(action, ''), COALESCE(outcome, ''), COALESCE(severity, ''),
			COALESCE(observer_vendor, ''), COALESCE(observer_product, ''), COALESCE(observer_hostname, ''), COALESCE(host_name, ''), COALESCE(user_name, ''),
			COALESCE(source_ip::text, ''), COALESCE(source_port, 0), COALESCE(destination_ip::text, ''), COALESCE(destination_port, 0),
			COALESCE(network_transport, ''), COALESCE(network_protocol, ''), COALESCE(rule_identifier, ''), COALESCE(rule_name, ''), COALESCE(message, ''), attributes
		FROM security_events WHERE project_id = $1 AND environment_id = $2 AND id = $3`, projectID, environmentID, eventID)
	event, scanError := scanSecurityEvent(row)
	if errors.Is(scanError, pgx.ErrNoRows) {
		return security.Event{}, nil
	}
	if scanError != nil {
		return security.Event{}, fmt.Errorf("get security event: %w", scanError)
	}
	return event, nil
}

type securityEventScanner interface{ Scan(...any) error }

func scanSecurityEvent(row securityEventScanner) (security.Event, error) {
	var event security.Event
	var attributes []byte
	if scanError := row.Scan(&event.ID, &event.RawEventID, &event.SourceID, &event.OccurredAt, &event.Format, &event.Kind, &event.Category, &event.Action, &event.Outcome, &event.Severity,
		&event.ObserverVendor, &event.ObserverProduct, &event.ObserverHostname, &event.HostName, &event.UserName, &event.SourceIP, &event.SourcePort, &event.DestinationIP, &event.DestinationPort,
		&event.NetworkTransport, &event.NetworkProtocol, &event.RuleIdentifier, &event.RuleName, &event.Message, &attributes); scanError != nil {
		return security.Event{}, scanError
	}
	if unmarshalError := json.Unmarshal(attributes, &event.Attributes); unmarshalError != nil {
		return security.Event{}, fmt.Errorf("decode security event attributes: %w", unmarshalError)
	}
	return event, nil
}
