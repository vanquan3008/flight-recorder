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
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/rollups"
	"github.com/vanquan3008/tracepilot/internal/storage/hybrid"
)

type incidentStore struct {
	store     *hybrid.Store
	ingestion config.IngestionConfig
}

func (store incidentStore) PersistObservabilityIncident(ctx context.Context, service string, end time.Time, evidence []analysis.Evidence) error {
	return store.store.PersistObservabilityIncident(ctx, store.ingestion, service, end, evidence)
}
func detectionServiceFor(store *hybrid.Store, scope config.IngestionConfig) *detection.Service {
	bind := func(detect func(context.Context, config.IngestionConfig, time.Time) ([]analysis.DetectedEvidence, error)) detection.Detector {
		return detection.DetectorFunc(func(ctx context.Context, end time.Time) ([]analysis.DetectedEvidence, error) {
			return detect(ctx, scope, end)
		})
	}
	return detection.NewService(incidentStore{store: store, ingestion: scope},
		bind(store.DetectServiceEvidence), bind(store.DetectDependencyEvidence), bind(store.DetectMetricEvidence))
}

const maxConcurrentDetection = 4

// rollupTimeout bounds the minute rollup refresh that runs before each detection pass.
const rollupTimeout = 20 * time.Second

// refreshRollups repeats refresh passes until one returns less than a full
// batch, so a backlog larger than one batch (many active environments, or a
// catch-up after downtime) drains within one run instead of growing by the
// excess every minute. ctx bounds the total time.
func refreshRollups(ctx context.Context, refresh func(context.Context) (int, error), batch int) (int, error) {
	total := 0
	for {
		count, err := refresh(ctx)
		total += count
		if err != nil || count < batch {
			return total, err
		}
		if ctx.Err() != nil {
			return total, ctx.Err()
		}
	}
}

func buildObservabilityDetectionFunc(scopes *scopeCatalog, store *hybrid.Store, selfMetrics *metrics.Registry) func(context.Context) (int, error) {
	return func(ctx context.Context) (int, error) {
		startedAt := time.Now()
		rollupCtx, cancelRollup := context.WithTimeout(ctx, rollupTimeout)
		minutes, rollupError := refreshRollups(rollupCtx, store.RefreshSpanRollups, rollups.Batch)
		cancelRollup()
		selfMetrics.ObserveSpanRollup(minutes, rollupError != nil)
		all, listError := scopes.All(ctx)
		if listError != nil {
			selfMetrics.ObserveIncidentDetection(time.Since(startedAt), 0, true)
			return 0, errors.Join(rollupError, listError)
		}
		if len(all) == 0 {
			selfMetrics.ObserveIncidentDetection(time.Since(startedAt), 0, rollupError != nil)
			return 0, rollupError
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
			runError = rollupError
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
					count, evaluateError := detectionServiceFor(store, scope).Run(scopeCtx, now)
					cancel()
					mu.Lock()
					total += count
					runError = errors.Join(runError, evaluateError)
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		selfMetrics.ObserveIncidentDetection(time.Since(startedAt), total, runError != nil)
		return total, runError
	}
}
