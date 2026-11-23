package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/vanquan3008/tracepilot/internal/application/replay"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	ingestotlp "github.com/vanquan3008/tracepilot/internal/ingest/otlp"
	"github.com/vanquan3008/tracepilot/internal/ingest/pipeline"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
)

const staleReplayCleanupTimeout = 5 * time.Second

// buildMetricsPipeline constructs the metrics telemetry ingestion pipeline.
func buildMetricsPipeline(
	cfg config.Config,
	storeMetrics func(context.Context, config.IngestionConfig, []telemetry.MetricDataPoint) error,
	catalog *scopeCatalog,
	selfMetrics *metrics.Registry,
) *pipeline.MetricsPipeline {
	return pipeline.NewMetricsPipeline(
		pipeline.MetricsPipelineConfig{
			QueueSize:     cfg.Ingestion.QueueSize,
			QueueMaxBytes: cfg.Ingestion.QueueMaxBytes,
			BatchSize:     cfg.Ingestion.BatchSize,
			Workers:       cfg.Ingestion.Workers,
			Metrics:       selfMetrics,
		},
		storeMetricsByScope(catalog, storeMetrics),
	)
}

// buildIngestionPipeline constructs the event telemetry ingestion pipeline. It takes the
// concrete writer because the hybrid store's promoted StoreEvents would refuse logs.
func buildIngestionPipeline(
	cfg config.Config,
	writer eventWriter,
	catalog *scopeCatalog,
	selfMetrics *metrics.Registry,
) *pipeline.Pipeline {
	return pipeline.New(
		pipeline.Config{
			QueueSize:      cfg.Ingestion.QueueSize,
			QueueMaxBytes:  cfg.Ingestion.QueueMaxBytes,
			BatchSize:      cfg.Ingestion.BatchSize,
			Workers:        cfg.Ingestion.Workers,
			EnqueueTimeout: cfg.Ingestion.EnqueueTimeout,
			BatchTimeout:   cfg.Ingestion.BatchTimeout,
			FlushInterval:  cfg.Ingestion.FlushInterval,
		},
		storeEventsByScope(catalog, writer.StoreEvents),
		selfMetrics,
	)
}

// buildReplaySubsystem initializes the replay service and cleans up any stale executions.
func buildReplaySubsystem(
	ctx context.Context,
	cfg config.Config,
	database *postgres.Database,
	logger *slog.Logger,
) (*replay.Service, error) {
	if !cfg.Replay.Enabled {
		return nil, nil
	}

	policy, err := replay.NewPolicy(cfg.Replay.AllowedHosts, cfg.Replay.AllowPrivate)
	if err != nil {
		logger.ErrorContext(ctx, "replay policy initialization failed", "error", err)
		return nil, fmt.Errorf("init replay policy: %w", err)
	}

	executor, err := replay.NewExecutor(policy, cfg.Replay.Timeout, cfg.Replay.MaxResponseBytes, cfg.Replay.MaxConcurrent)
	if err != nil {
		logger.ErrorContext(ctx, "replay executor initialization failed", "error", err)
		return nil, fmt.Errorf("init replay executor: %w", err)
	}

	cleanupCtx, cancel := context.WithTimeout(ctx, staleReplayCleanupTimeout)
	defer cancel()

	if failedCount, err := database.FailStaleReplayExecutions(cleanupCtx, cfg.Replay.Timeout); err != nil {
		logger.WarnContext(ctx, "stale replay execution cleanup failed", "error", err)
	} else if failedCount > 0 {
		logger.InfoContext(ctx, "marked stale replay executions as failed", "count", failedCount)
	}

	return replay.NewService(database, executor), nil
}

// buildOTLPReceiver configures and returns the OTLP ingestion receiver with token authentication.
func buildOTLPReceiver(
	cfg config.Config,
	pipe *pipeline.Pipeline,
	metricsPipe *pipeline.MetricsPipeline,
	selfMetrics *metrics.Registry,
	tokenSource auth.IngestTokenSource,
	catalog *scopeCatalog,
) *ingestotlp.Receiver {
	secret := []byte(cfg.Auth.EffectiveIngestJWTSecret())
	nowFn := time.Now

	authenticateOTLP := func(ctx context.Context, raw string) (auth.Principal, error) {
		return auth.AuthenticateIngestToken(ctx, secret, tokenSource, raw, nowFn())
	}

	return ingestotlp.NewReceiver(cfg.Ingestion, pipe, metricsPipe, selfMetrics, authenticateOTLP).
		WithScopes(catalog.Resolve).
		WithTrustedProxies(cfg.HTTP.TrustedProxyPrefixes())
}

// readyReporter reports whether a component is ready to accept work.
type readyReporter interface {
	Ready() bool
}

// namedReadyComponent pairs a readiness check target with a human-readable identifier.
type namedReadyComponent struct {
	name     string
	reporter readyReporter
}

// databaseProbe checks that the database answers and its schema is current.
type databaseProbe interface {
	Ready(context.Context) error
}

// buildReadinessCheck verifies that all core components and the database are ready.
func buildReadinessCheck(database databaseProbe, components ...namedReadyComponent) func(context.Context) error {
	return func(ctx context.Context) error {
		for _, comp := range components {
			if comp.reporter == nil {
				return fmt.Errorf("component %q is nil", comp.name)
			}
			if !comp.reporter.Ready() {
				return fmt.Errorf("component %q is not ready", comp.name)
			}
		}

		if database == nil {
			return fmt.Errorf("database probe is nil")
		}
		if err := database.Ready(ctx); err != nil {
			return fmt.Errorf("database probe failed: %w", err)
		}

		return nil
	}
}

// logStartup outputs the application startup banner and configuration warnings.
func logStartup(logger *slog.Logger, cfg config.Config, version string) {
	if logger == nil {
		return
	}

	logger.Info("Starting TracePilot",
		"version", version,
		"http_address", cfg.HTTP.Address,
		"otlp_grpc_address", cfg.Ingestion.GRPCAddress,
		"otlp_http_address", cfg.Ingestion.HTTPAddress,
		"replay_enabled", cfg.Replay.Enabled,
	)

	if cfg.TLS.CertificateFile == "" {
		logger.Warn("OTLP listeners are plaintext; use TLS or terminate TLS before TracePilot")
	}

	if !cfg.Auth.AllowRegistration {
		logger.Info("registration is closed; only the first account and accounts made by a super administrator exist")
	}
}
