package main

import (
	"context"
	"errors"
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

// detectionServiceFor wires the detectors and the incident store to one project environment.
func detectionServiceFor(database *postgres.Database, scope config.IngestionConfig) *detection.Service {
	bind := func(detect func(context.Context, config.IngestionConfig, time.Time) ([]analysis.DetectedEvidence, error)) detection.Detector {
		return detection.DetectorFunc(func(ctx context.Context, end time.Time) ([]analysis.DetectedEvidence, error) {
			return detect(ctx, scope, end)
		})
	}
	return detection.NewService(incidentStore{database: database, ingestion: scope},
		bind(database.DetectServiceEvidence), bind(database.DetectDependencyEvidence), bind(database.DetectMetricEvidence))
}

// buildObservabilityDetectionFunc runs detection in every project environment. One that fails does
// not stop the rest: its error is reported after the others have been evaluated.
func buildObservabilityDetectionFunc(scopes *scopeCatalog, database *postgres.Database, selfMetrics *metrics.Registry) func(context.Context) (int, error) {
	return func(ctx context.Context) (int, error) {
		startedAt := time.Now()
		all, listError := scopes.All(ctx)
		if listError != nil {
			selfMetrics.ObserveIncidentDetection(time.Since(startedAt), 0, true)
			return 0, listError
		}
		total, runError := 0, error(nil)
		for _, scope := range all {
			count, evaluateError := detectionServiceFor(database, scope).Run(ctx, time.Now())
			total += count
			runError = errors.Join(runError, evaluateError)
		}
		selfMetrics.ObserveIncidentDetection(time.Since(startedAt), total, runError != nil)
		return total, runError
	}
}
