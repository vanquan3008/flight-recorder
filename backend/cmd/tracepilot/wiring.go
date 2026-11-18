package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/vanquan3008/tracepilot/internal/application/replay"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
	ingestotlp "github.com/vanquan3008/tracepilot/internal/ingest/otlp"
	"github.com/vanquan3008/tracepilot/internal/ingest/pipeline"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
)

// buildMetricsPipeline constructs the metrics telemetry ingestion pipeline.
func buildMetricsPipeline(cfg config.Config, database *postgres.Database, catalog *scopeCatalog, selfMetrics *metrics.Registry) *pipeline.MetricsPipeline {
	return pipeline.NewMetricsPipeline(
		pipeline.MetricsPipelineConfig{
			QueueSize:     cfg.Ingestion.QueueSize,
			QueueMaxBytes: cfg.Ingestion.QueueMaxBytes,
			BatchSize:     cfg.Ingestion.BatchSize,
			Workers:       cfg.Ingestion.Workers,
			Metrics:       selfMetrics,
		},
		storeMetricsByScope(catalog, database.StoreMetrics),
	)
}

// buildIngestionPipeline constructs the event telemetry ingestion pipeline. It takes the
// concrete writer because the hybrid store's promoted StoreEvents would refuse logs.
func buildIngestionPipeline(cfg config.Config, writer eventWriter, catalog *scopeCatalog, selfMetrics *metrics.Registry) *pipeline.Pipeline {
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
func buildReplaySubsystem(ctx context.Context, cfg config.Config, database *postgres.Database, logger *slog.Logger) (*replay.Service, error) {
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

	// Sử dụng defer để đảm bảo cancelCleanup luôn được gọi, tránh rò rỉ context resource
	cleanupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if _, err := database.FailStaleReplayExecutions(cleanupCtx, cfg.Replay.Timeout); err != nil {
		logger.WarnContext(ctx, "stale replay execution cleanup failed", "error", err)
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
	// Telemetry senders present an ingest token: a JWT for one project that its owner can revoke.
	secret := []byte(cfg.Auth.EffectiveIngestJWTSecret())

	authenticateOTLP := func(ctx context.Context, raw string) (auth.Principal, error) {
		return auth.AuthenticateIngestToken(ctx, secret, tokenSource, raw, time.Now())
	}

	return ingestotlp.NewReceiver(cfg.Ingestion, pipe, metricsPipe, selfMetrics, authenticateOTLP).
		WithScopes(catalog.Resolve).
		WithTrustedProxies(cfg.HTTP.TrustedProxyPrefixes())
}

// readyReporter is anything that can report whether it accepts work.
type readyReporter interface {
	Ready() bool
}

// databaseProbe checks that the database answers and its schema is current.
type databaseProbe interface {
	Ready(context.Context) error
}

// buildReadinessCheck verifies that both the database and all core ingestion components are ready.
func buildReadinessCheck(database databaseProbe, components ...readyReporter) func(context.Context) error {
	return func(ctx context.Context) error {
		for i, component := range components {
			if !component.Ready() {
				return fmt.Errorf("ingestion component at index %d is not ready", i)
			}
		}
		if err := database.Ready(ctx); err != nil {
			return fmt.Errorf("database probe failed: %w", err)
		}
		return nil
	}
}

// logStartup outputs the application startup banner and configuration warnings.
func logStartup(logger *slog.Logger, cfg config.Config, version string) {
	logger.Info("Starting TracePilot",
		"version", version,
		"http_address", cfg.HTTP.Address,
		"otlp_grpc_address", cfg.Ingestion.GRPCAddress,
		"otlp_http_address", cfg.Ingestion.HTTPAddress,
	)

	if cfg.TLS.CertificateFile == "" {
		logger.Warn("OTLP listeners are plaintext; use TLS or terminate TLS before TracePilot")
	}

	if !cfg.Auth.AllowRegistration {
		logger.Info("registration is closed; only the first account and accounts made by a super administrator exist")
	}
}
