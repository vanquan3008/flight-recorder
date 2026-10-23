package main

import (
	"context"
	"time"

	"github.com/vanquan3008/tracepilot/internal/application/detection"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
)

// incidentStore binds the configured ingestion scope to the storage adapter so
// the application workflow stays free of configuration types.
type incidentStore struct {
	database  *postgres.Database
	ingestion config.IngestionConfig
}

func (store incidentStore) PersistObservabilityIncident(ctx context.Context, service string, end time.Time, evidence []analysis.Evidence) error {
	return store.database.PersistObservabilityIncident(ctx, store.ingestion, service, end, evidence)
}

func buildObservabilityDetectionFunc(cfg config.Config, database *postgres.Database, selfMetrics *metrics.Registry) func(context.Context) (int, error) {
	scope := cfg.Ingestion
	bind := func(detect func(context.Context, config.IngestionConfig, time.Time) ([]analysis.DetectedEvidence, error)) detection.Detector {
		return detection.DetectorFunc(func(ctx context.Context, end time.Time) ([]analysis.DetectedEvidence, error) {
			return detect(ctx, scope, end)
		})
	}
	service := detection.NewService(incidentStore{database: database, ingestion: scope},
		bind(database.DetectServiceEvidence), bind(database.DetectDependencyEvidence), bind(database.DetectMetricEvidence))
	return func(ctx context.Context) (int, error) {
		startedAt := time.Now()
		count, evaluateError := service.Run(ctx, time.Now())
		selfMetrics.ObserveIncidentDetection(time.Since(startedAt), count, evaluateError != nil)
		return count, evaluateError
	}
}
