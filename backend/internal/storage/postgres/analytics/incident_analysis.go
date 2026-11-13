package analytics

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
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/incidents"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/outbox"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres/pgvalue"
	sqlcstore "github.com/vanquan3008/tracepilot/internal/storage/postgres/sqlc"
)

const (
	observabilityOperation = "observability baseline"
	observabilityErrorType = "OBSERVABILITY_ANOMALY"
	observabilityMessage   = "Telemetry deviated from its baseline"
)

// PersistObservabilityIncident atomically stores the detector event, incident,
// evidence and outbox entry for one service.
func (store *AnalyticsStore) PersistObservabilityIncident(ctx context.Context, ingestion config.IngestionConfig, service string, end time.Time, evidence []analysis.Evidence) error {
	eventID := telemetry.DeterministicID(end, "observability-detection", ingestion.ProjectID, ingestion.EnvironmentID, service)
	attributes, marshalError := json.Marshal(map[string]any{"detector": "baseline", "evidence_count": len(evidence), "window_minutes": 5})
	if marshalError != nil {
		return fmt.Errorf("encode observability attributes: %w", marshalError)
	}
	persistError := pgx.BeginFunc(ctx, store.pool, func(transaction pgx.Tx) error {
		queries := store.queries.WithTx(transaction)
		if err := queries.EnsureService(ctx, sqlcstore.EnsureServiceParams{
			ID:        telemetry.DeterministicID(end, ingestion.EnvironmentID, service),
			ProjectID: ingestion.ProjectID, EnvironmentID: ingestion.EnvironmentID, Name: service,
		}); err != nil {
			return fmt.Errorf("ensure detected service: %w", err)
		}
		if err := queries.InsertObservabilityEvent(ctx, sqlcstore.InsertObservabilityEventParams{
			ID: eventID, ProjectID: ingestion.ProjectID, EnvironmentID: ingestion.EnvironmentID,
			ServiceName: service, OccurredAt: end, Attributes: attributes,
			Operation: pgvalue.OrNil(observabilityOperation), ErrorType: pgvalue.OrNil(observabilityErrorType),
			ErrorMessage: pgvalue.OrNil(observabilityMessage),
		}); err != nil {
			return fmt.Errorf("insert observability event: %w", err)
		}
		incidentID, created, err := incidents.RecordOccurrence(ctx, transaction, ingestion, telemetry.Event{
			ID: eventID, ProjectID: ingestion.ProjectID, Environment: ingestion.EnvironmentName,
			ServiceName: service, Type: telemetry.EventCustom, Timestamp: end, Severity: "ERROR", Status: "ERROR",
			Operation: observabilityOperation, ErrorType: observabilityErrorType, ErrorMessage: observabilityMessage,
			Attributes: map[string]any{"detector": "baseline"},
		})
		if err != nil {
			return err
		}
		if incidentID == "" {
			// A retry of the same detection: reuse the incident it opened.
			incidentID, err = queries.GetOccurrenceIncidentID(ctx, sqlcstore.GetOccurrenceIncidentIDParams{ProjectID: ingestion.ProjectID, EventID: eventID})
			if err != nil {
				return fmt.Errorf("find idempotent observability incident: %w", err)
			}
		}
		for _, item := range evidence {
			if err := queries.InsertDetectedEvidence(ctx, sqlcstore.InsertDetectedEvidenceParams{
				ID:        telemetry.DeterministicID(end, "incident-evidence", incidentID, item.SignalType, item.EntityID, item.MetricName, item.SeriesID),
				ProjectID: ingestion.ProjectID, EnvironmentID: ingestion.EnvironmentID, IncidentID: incidentID,
				SignalType: item.SignalType, EntityType: item.EntityType, EntityID: item.EntityID,
				MetricName: item.MetricName, Direction: item.Direction, ObservedValue: item.ObservedValue,
				BaselineValue: item.BaselineValue, ChangeRatio: item.ChangeRatio,
				WindowStartedAt: item.WindowStartedAt, WindowEndedAt: item.WindowEndedAt,
				TraceIDs: nonNilStrings(item.TraceIDs), Summary: item.Summary, SeriesID: item.SeriesID,
			}); err != nil {
				return fmt.Errorf("insert incident evidence: %w", err)
			}
		}
		if !created {
			return nil
		}
		return outbox.EnqueueDeliveries(ctx, transaction, ingestion.ProjectID, "incident.opened", map[string]any{
			"incident_id": incidentID, "environment_id": ingestion.EnvironmentID, "service": service,
			"title": service + " observability baseline failed", "severity": "ERROR",
		})
	})
	if persistError != nil {
		return fmt.Errorf("persist observability incident: %w", persistError)
	}
	return nil
}

// GetIncidentAnalysis ranks RCA hypotheses from the incident's latest evidence
// and the dependencies observed in the same window. A missing incident yields
// an empty analysis.
func (store *AnalyticsStore) GetIncidentAnalysis(ctx context.Context, projectID, environmentID, incidentID string) (analysis.IncidentAnalysis, error) {
	service, err := store.queries.GetIncidentPrimaryService(ctx, sqlcstore.GetIncidentPrimaryServiceParams{ProjectID: projectID, EnvironmentID: environmentID, ID: incidentID})
	if errors.Is(err, pgx.ErrNoRows) {
		return analysis.IncidentAnalysis{}, nil
	}
	if err != nil {
		return analysis.IncidentAnalysis{}, fmt.Errorf("load incident for analysis: %w", err)
	}
	rows, err := store.queries.ListLatestIncidentEvidence(ctx, sqlcstore.ListLatestIncidentEvidenceParams{ProjectID: projectID, EnvironmentID: environmentID, IncidentID: incidentID})
	if err != nil {
		return analysis.IncidentAnalysis{}, fmt.Errorf("list incident evidence: %w", err)
	}
	evidence := make([]analysis.Evidence, 0, len(rows))
	for _, row := range rows {
		evidence = append(evidence, analysis.Evidence{
			ID: row.ID, SeriesID: row.SeriesID, SignalType: row.SignalType, EntityType: row.EntityType,
			EntityID: row.EntityID, MetricName: row.MetricName, Direction: row.Direction,
			ObservedValue: row.ObservedValue, BaselineValue: row.BaselineValue, ChangeRatio: row.ChangeRatio,
			WindowStartedAt: row.WindowStartedAt, WindowEndedAt: row.WindowEndedAt,
			TraceIDs: row.TraceIDs, Summary: row.Summary,
		})
	}
	dependencies := []analysis.TopologyEdge{}
	if len(evidence) > 1 {
		from, to := evidenceWindow(evidence)
		topology, err := store.ListTopology(ctx, projectID, analysis.Filter{EnvironmentID: environmentID, From: from, To: to})
		if err != nil {
			return analysis.IncidentAnalysis{}, err
		}
		dependencies = topology.Edges
	}
	return analysis.BuildRCA(incidentID, service, evidence, dependencies), nil
}

// evidenceWindow spans every evidence window, widened to one detection window
// when they all end at the same instant.
func evidenceWindow(evidence []analysis.Evidence) (time.Time, time.Time) {
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
	return from, to
}

// nonNilStrings stores a missing list as an empty array; the column is NOT NULL.
func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
