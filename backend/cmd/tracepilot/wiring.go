package main

import (
	"context"
	"errors"
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

func buildIngestionPipeline(cfg config.Config, database *postgres.Database, catalog *scopeCatalog, selfMetrics *metrics.Registry) *pipeline.Pipeline {
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
		storeEventsByScope(catalog, database.StoreEvents),
		selfMetrics,
	)
}

func buildReplaySubsystem(ctx context.Context, cfg config.Config, database *postgres.Database, logger *slog.Logger) (*replay.Service, error) {
	if !cfg.Replay.Enabled {
		return nil, nil
	}
	policy, policyError := replay.NewPolicy(cfg.Replay.AllowedHosts, cfg.Replay.AllowPrivate)
	if policyError != nil {
		logger.ErrorContext(ctx, "replay policy initialization failed", "error", policyError)
		return nil, policyError
	}
	executor, executorError := replay.NewExecutor(policy, cfg.Replay.Timeout, cfg.Replay.MaxResponseBytes, cfg.Replay.MaxConcurrent)
	if executorError != nil {
		logger.ErrorContext(ctx, "replay executor initialization failed", "error", executorError)
		return nil, executorError
	}
	cleanupCtx, cancelCleanup := context.WithTimeout(ctx, 5*time.Second)
	if _, cleanupError := database.FailStaleReplayExecutions(cleanupCtx, cfg.Replay.Timeout); cleanupError != nil {
		logger.WarnContext(ctx, "stale replay execution cleanup failed", "error", cleanupError)
	}
	cancelCleanup()
	return replay.NewService(database, executor), nil
}

func buildOTLPReceiver(
	cfg config.Config,
	pipe *pipeline.Pipeline,
	metricsPipe *pipeline.MetricsPipeline,
	selfMetrics *metrics.Registry,
	tokenSource auth.IngestTokenSource, catalog *scopeCatalog) *ingestotlp.Receiver {
	// Telemetry senders present an ingest token: a JWT for one project that its owner can revoke.
	secret := []byte(cfg.Auth.EffectiveIngestJWTSecret())
	authenticateOTLP := func(ctx context.Context, raw string) (auth.Principal, error) {
		return auth.AuthenticateIngestToken(ctx, secret, tokenSource, raw, time.Now())
	}
	return ingestotlp.NewReceiver(cfg.Ingestion, pipe, metricsPipe, selfMetrics, authenticateOTLP).
		WithScopes(catalog.Resolve).
		WithTrustedProxies(cfg.HTTP.TrustedProxyPrefixes())
}

// readyReporter is anything that can say whether it accepts work.
type readyReporter interface{ Ready() bool }

// databaseProbe checks that the database answers and its schema is current.
type databaseProbe interface{ Ready(context.Context) error }

// buildReadinessCheck is ready only when every ingestion component is: each
// pipeline and listener can stop on its own, and a silent one would leave the
// service reporting healthy while it rejects part of its traffic.
func buildReadinessCheck(database databaseProbe, components ...readyReporter) func(context.Context) error {
	return func(ctx context.Context) error {
		for _, component := range components {
			if !component.Ready() {
				return errors.New("ingestion runtime is not ready")
			}
		}
		return database.Ready(ctx)
	}
}

func logStartup(logger *slog.Logger, cfg config.Config) {
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
