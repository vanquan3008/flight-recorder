// Package hybrid combines the PostgreSQL store with the ClickHouse telemetry store.
// The two adapters must not import each other, so their composition lives here.
package hybrid

import (
	"context"
	"fmt"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/analysis"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
	"golang.org/x/sync/errgroup"
)

type clickHouseStore interface {
	StoreLogs(context.Context, string, string, []telemetry.Event) error
	StoreSpans(context.Context, string, string, []telemetry.Event) error
	StoreMetrics(context.Context, string, string, []telemetry.MetricDataPoint) error
	ListLogs(context.Context, string, query.LogFilter) ([]query.LogRecord, error)
	GetLogSince(context.Context, string, string, string, time.Time) (query.LogRecord, error)
	LogQuality(context.Context, string, string, string, time.Time, time.Time) (int64, int64, error)
	ExpireProjectTelemetry(context.Context, string, time.Time) error
	DropPartitionsBefore(context.Context, time.Time) error
	ListTraces(context.Context, string, query.TraceFilter) ([]query.TraceSummary, error)
	GetTrace(context.Context, string, string, string, query.SpanPage) ([]query.SpanRecord, bool, error)
	ListServices(context.Context, string, string, time.Time, time.Time) ([]query.ServiceSummary, error)
	ListServiceDependencies(context.Context, string, string, string) ([]query.ServiceDependency, error)
	ListTopology(context.Context, string, analysis.Filter) (analysis.Topology, error)
	ListMetricPoints(context.Context, string, analysis.Filter) ([]analysis.MetricPoint, error)
	SignalQuality(context.Context, string, analysis.Filter) (int64, int64, int64, error)
	RequestPerformance(context.Context, string, query.PerformanceFilter) (query.RequestPerformance, error)
	DetectServiceEvidence(context.Context, config.IngestionConfig, time.Time) ([]analysis.DetectedEvidence, error)
	DetectDependencyEvidence(context.Context, config.IngestionConfig, time.Time) ([]analysis.DetectedEvidence, error)
	DetectMetricEvidence(context.Context, config.IngestionConfig, time.Time) ([]analysis.DetectedEvidence, error)
	RefreshSpanRollups(context.Context) (int, error)
	Ready(context.Context) error
	Close()
}

// Store exposes every PostgreSQL method plus the telemetry ClickHouse serves.
// Its promoted StoreEvents writes incident envelopes to PostgreSQL only.
type Store struct {
	*postgres.Database
	ch                  clickHouseStore
	defaultRetentionAge time.Duration
	retention           *retentionCache
}

func (store *Store) Close() {
	store.ch.Close()
	store.Database.Close()
}

// Ready checks both databases concurrently; the first failure cancels the other probe.
func (store *Store) Ready(ctx context.Context) error {
	return checkDatabases(ctx, store.Database, store.ch)
}

func (store *Store) StoreLogs(ctx context.Context, project, environment string, logs []telemetry.Event) error {
	return store.ch.StoreLogs(ctx, project, environment, logs)
}

func (store *Store) StoreSpans(ctx context.Context, project, environment string, spans []telemetry.Event) error {
	return store.ch.StoreSpans(ctx, project, environment, spans)
}

func (store *Store) StoreMetrics(ctx context.Context, ingestion config.IngestionConfig, points []telemetry.MetricDataPoint) error {
	return store.ch.StoreMetrics(ctx, ingestion.ProjectID, ingestion.EnvironmentID, points)
}

func (store *Store) ListLogs(ctx context.Context, project string, filter query.LogFilter) ([]query.LogRecord, error) {
	cutoff, err := store.retentionCutoff(ctx, project)
	if err != nil {
		return nil, err
	}
	filter.From = maxTime(filter.From, cutoff)
	return store.ch.ListLogs(ctx, project, filter)
}

func (store *Store) GetLog(ctx context.Context, project, environment, id string) (query.LogRecord, error) {
	cutoff, err := store.retentionCutoff(ctx, project)
	if err != nil {
		return query.LogRecord{}, err
	}
	return store.ch.GetLogSince(ctx, project, environment, id, cutoff)
}

func (store *Store) UpdateProjectRetention(ctx context.Context, projectID string, days *int32, actor auth.Actor) error {
	err := store.Database.UpdateProjectRetention(ctx, projectID, days, actor)
	store.retention.forget(projectID)
	return err
}

func (store *Store) retentionCutoff(ctx context.Context, project string) (time.Time, error) {
	days, err := store.retention.get(ctx, project, store.GetProjectRetention)
	if err != nil {
		return time.Time{}, err
	}
	if age := telemetry.RetentionAge(days, store.defaultRetentionAge); age > 0 {
		cutoff := time.Now().UTC().Add(-age)
		if cutoff.After(time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)) {
			return cutoff, nil
		}
	}
	return time.Time{}, nil
}

// DropExpiredClickHousePartitions drops ClickHouse partitions every project's
// policy has expired, under the same policy lock as PostgreSQL partition drops.
func (store *Store) DropExpiredClickHousePartitions(ctx context.Context, now time.Time, fallback time.Duration) error {
	return store.WithLongestRetention(ctx, now, fallback, func(before time.Time) error {
		return store.ch.DropPartitionsBefore(ctx, before)
	})
}

// PruneProjectLogs expires every ClickHouse telemetry table, not only logs.
func (store *Store) PruneProjectLogs(ctx context.Context, projectID string, now time.Time, fallback time.Duration) error {
	return store.WithProjectRetention(ctx, projectID, now, fallback, func(before time.Time) error {
		return store.ch.ExpireProjectTelemetry(ctx, projectID, before)
	})
}

type databaseProbe interface {
	Ready(context.Context) error
}

func checkDatabases(ctx context.Context, database, clickHouse databaseProbe) error {
	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		if err := database.Ready(ctx); err != nil {
			return fmt.Errorf("PostgreSQL readiness: %w", err)
		}
		return nil
	})
	group.Go(func() error {
		if err := clickHouse.Ready(ctx); err != nil {
			return fmt.Errorf("ClickHouse readiness: %w", err)
		}
		return nil
	})
	return group.Wait()
}
