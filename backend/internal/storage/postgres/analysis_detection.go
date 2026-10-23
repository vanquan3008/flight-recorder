package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

// PersistObservabilityIncident atomically stores the detector event, incident, evidence and outbox entry for one service.
func (database *Database) PersistObservabilityIncident(ctx context.Context, ingestion config.IngestionConfig, service string, end time.Time, evidence []analysis.Evidence) error {
	transaction, err := database.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin observability incident: %w", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	serviceID := telemetry.DeterministicID(end, ingestion.EnvironmentID, service)
	if _, err := transaction.Exec(ctx, `INSERT INTO services (id,project_id,environment_id,name) VALUES ($1,$2,$3,$4) ON CONFLICT (project_id,environment_id,name) DO NOTHING`, serviceID, ingestion.ProjectID, ingestion.EnvironmentID, service); err != nil {
		return fmt.Errorf("ensure detected service: %w", err)
	}
	eventID := telemetry.DeterministicID(end, "observability-detection", ingestion.ProjectID, ingestion.EnvironmentID, service)
	attributes, _ := json.Marshal(map[string]any{"detector": "baseline", "evidence_count": len(evidence), "window_minutes": 5})
	if _, err := transaction.Exec(ctx, `
		INSERT INTO events (id,project_id,environment_id,service_name,event_type,occurred_at,severity,status,operation,error_type,error_message,attributes)
		VALUES ($1,$2,$3,$4,'CUSTOM',$5,'ERROR','ERROR','observability baseline','OBSERVABILITY_ANOMALY','Telemetry deviated from its baseline',$6)
		ON CONFLICT (id, occurred_at) DO NOTHING`, eventID, ingestion.ProjectID, ingestion.EnvironmentID, service, end, attributes); err != nil {
		return fmt.Errorf("insert observability event: %w", err)
	}
	event := telemetry.Event{ID: eventID, ProjectID: ingestion.ProjectID, Environment: ingestion.EnvironmentName, ServiceName: service, Type: telemetry.EventCustom, Timestamp: end, Severity: "ERROR", Status: "ERROR", Operation: "observability baseline", ErrorType: "OBSERVABILITY_ANOMALY", ErrorMessage: "Telemetry deviated from its baseline", Attributes: map[string]any{"detector": "baseline"}}
	incidentID, created, err := storeIncident(ctx, transaction, ingestion, event)
	if err != nil {
		return err
	}
	if incidentID == "" {
		err = transaction.QueryRow(ctx, `SELECT incident_id FROM incident_occurrences WHERE project_id=$1 AND event_id=$2`, ingestion.ProjectID, eventID).Scan(&incidentID)
		if err != nil {
			return fmt.Errorf("find idempotent observability incident: %w", err)
		}
	}
	for index := range evidence {
		item := &evidence[index]
		if item.TraceIDs == nil {
			item.TraceIDs = []string{}
		}
		item.ID = telemetry.DeterministicID(end, "incident-evidence", incidentID, item.SignalType, item.EntityID, item.MetricName, item.SeriesID)
		if _, err := transaction.Exec(ctx, `
			INSERT INTO incident_evidence (id,project_id,environment_id,incident_id,signal_type,entity_type,entity_id,metric_name,direction,observed_value,baseline_value,change_ratio,window_started_at,window_ended_at,trace_ids,summary,series_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
			ON CONFLICT DO NOTHING`, item.ID, ingestion.ProjectID, ingestion.EnvironmentID, incidentID, item.SignalType, item.EntityType, item.EntityID, item.MetricName, item.Direction, item.ObservedValue, item.BaselineValue, item.ChangeRatio, item.WindowStartedAt, item.WindowEndedAt, item.TraceIDs, item.Summary, item.SeriesID); err != nil {
			return fmt.Errorf("insert incident evidence: %w", err)
		}
	}
	if created {
		if err := enqueueWebhookDeliveries(ctx, transaction, ingestion.ProjectID, "incident.opened", map[string]any{"incident_id": incidentID, "environment_id": ingestion.EnvironmentID, "service": service, "title": service + " observability baseline failed", "severity": "ERROR"}); err != nil {
			return err
		}
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit observability incident: %w", err)
	}
	return nil
}

func (database *Database) GetIncidentAnalysis(ctx context.Context, projectID, environmentID, incidentID string) (analysis.IncidentAnalysis, error) {
	var service string
	err := database.pool.QueryRow(ctx, `SELECT primary_service FROM incidents WHERE project_id=$1 AND environment_id=$2 AND id=$3`, projectID, environmentID, incidentID).Scan(&service)
	if errors.Is(err, pgx.ErrNoRows) {
		return analysis.IncidentAnalysis{}, nil
	}
	if err != nil {
		return analysis.IncidentAnalysis{}, fmt.Errorf("load incident for analysis: %w", err)
	}
	rows, err := database.pool.Query(ctx, `
		WITH selected_window AS (
			SELECT max(window_ended_at) ended_at FROM incident_evidence
			WHERE project_id=$1 AND environment_id=$2 AND incident_id=$3
		)
		SELECT id,signal_type,entity_type,entity_id,metric_name,direction,observed_value,baseline_value,change_ratio,window_started_at,window_ended_at,trace_ids,summary,series_id
		FROM (
			SELECT DISTINCT ON (signal_type,entity_id,metric_name,series_id)
				id,signal_type,entity_type,entity_id,metric_name,direction,observed_value,baseline_value,change_ratio,window_started_at,window_ended_at,trace_ids,summary,series_id
			FROM incident_evidence CROSS JOIN selected_window
			WHERE project_id=$1 AND environment_id=$2 AND incident_id=$3
			  AND (window_ended_at=selected_window.ended_at OR
			       (signal_type='event' AND window_ended_at>=selected_window.ended_at-interval '5 minutes'))
			ORDER BY signal_type,entity_id,metric_name,series_id,window_ended_at DESC,id DESC
		) latest ORDER BY window_ended_at DESC,id DESC LIMIT 100`, projectID, environmentID, incidentID)
	if err != nil {
		return analysis.IncidentAnalysis{}, fmt.Errorf("list incident evidence: %w", err)
	}
	defer rows.Close()
	evidence := make([]analysis.Evidence, 0)
	for rows.Next() {
		var item analysis.Evidence
		if err := rows.Scan(&item.ID, &item.SignalType, &item.EntityType, &item.EntityID, &item.MetricName, &item.Direction, &item.ObservedValue, &item.BaselineValue, &item.ChangeRatio, &item.WindowStartedAt, &item.WindowEndedAt, &item.TraceIDs, &item.Summary, &item.SeriesID); err != nil {
			return analysis.IncidentAnalysis{}, fmt.Errorf("scan incident evidence: %w", err)
		}
		evidence = append(evidence, item)
	}
	if err := rows.Err(); err != nil {
		return analysis.IncidentAnalysis{}, fmt.Errorf("iterate incident evidence: %w", err)
	}
	rows.Close()
	dependencies := []analysis.TopologyEdge{}
	if len(evidence) > 1 {
		from, to := evidence[0].WindowStartedAt, evidence[0].WindowEndedAt
		for _, item := range evidence {
			if item.WindowStartedAt.Before(from) {
				from = item.WindowStartedAt
			}
			if item.WindowEndedAt.After(to) {
				to = item.WindowEndedAt
			}
		}
		if !from.Before(to) {
			from = to.Add(-currentWindow)
		}
		topology, err := database.ListTopology(ctx, projectID, analysis.Filter{EnvironmentID: environmentID, From: from, To: to})
		if err != nil {
			return analysis.IncidentAnalysis{}, err
		}
		dependencies = topology.Edges
	}
	return analysis.BuildRCA(incidentID, service, evidence, dependencies), nil
}
