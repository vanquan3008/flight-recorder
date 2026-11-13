package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/vanquan3008/tracepilot/internal/application/detection"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
)

type incidentStore struct {
	database  *postgres.Database
	ingestion config.IngestionConfig
}

func (store incidentStore) PersistObservabilityIncident(ctx context.Context, service string, end time.Time, evidence []analysis.Evidence) error {
	return store.database.PersistObservabilityIncident(ctx, store.ingestion, service, end, evidence)
}
func detectionServiceFor(database *postgres.Database, scope config.IngestionConfig) *detection.Service {
	bind := func(detect func(context.Context, config.IngestionConfig, time.Time) ([]analysis.DetectedEvidence, error)) detection.Detector {
		return detection.DetectorFunc(func(ctx context.Context, end time.Time) ([]analysis.DetectedEvidence, error) {
			return detect(ctx, scope, end)
		})
	}
	return detection.NewService(incidentStore{database: database, ingestion: scope},
		bind(database.DetectServiceEvidence), bind(database.DetectDependencyEvidence), bind(database.DetectMetricEvidence))
}

const maxConcurrentDetection = 4

func buildObservabilityDetectionFunc(scopes *scopeCatalog, database *postgres.Database, selfMetrics *metrics.Registry) func(context.Context) (int, error) {
	return func(ctx context.Context) (int, error) {
		startedAt := time.Now()
		all, listError := scopes.All(ctx)
		if listError != nil {
			selfMetrics.ObserveIncidentDetection(time.Since(startedAt), 0, true)
			return 0, listError
		}
		if len(all) == 0 {
			selfMetrics.ObserveIncidentDetection(time.Since(startedAt), 0, false)
			return 0, nil
		}
		workerCount := min(len(all), maxConcurrentDetection)
		jobs := make(chan config.IngestionConfig, len(all))
		for _, scope := range all {
			jobs <- scope
		}
		close(jobs)

		var (
			mu       sync.Mutex
			total    int
			runError error
			wg       sync.WaitGroup
			now      = time.Now()
		)
		for range workerCount {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for scope := range jobs {
					if ctx.Err() != nil {
						return
					}
					scopeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
					rollupCtx, cancelRollup := context.WithTimeout(scopeCtx, 5*time.Second)
					rollupError := database.RefreshSpanRollups(rollupCtx, scope)
					cancelRollup()
					count, evaluateError := detectionServiceFor(database, scope).Run(scopeCtx, now)
					cancel()
					mu.Lock()
					total += count
					runError = errors.Join(runError, rollupError, evaluateError)
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		selfMetrics.ObserveIncidentDetection(time.Since(startedAt), total, runError != nil)
		return total, runError
	}
}
