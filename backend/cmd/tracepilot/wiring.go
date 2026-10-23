package main

import (
	"context"
	"errors"
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

func buildMetricsPipeline(cfg config.Config, database *postgres.Database, selfMetrics *metrics.Registry) *pipeline.MetricsPipeline {
	return pipeline.NewMetricsPipeline(
		pipeline.MetricsPipelineConfig{
			QueueSize:     cfg.Ingestion.QueueSize,
			QueueMaxBytes: cfg.Ingestion.QueueMaxBytes,
			BatchSize:     cfg.Ingestion.BatchSize,
			Workers:       cfg.Ingestion.Workers,
			Metrics:       selfMetrics,
		},
		func(ctx context.Context, points []telemetry.MetricDataPoint) error {
			return database.StoreMetrics(ctx, cfg.Ingestion, points)
		},
	)
}

func buildIngestionPipeline(cfg config.Config, database *postgres.Database, selfMetrics *metrics.Registry) *pipeline.Pipeline {
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
		func(storeCtx context.Context, events []telemetry.Event) error {
			return database.StoreEvents(storeCtx, cfg.Ingestion, events)
		},
		selfMetrics,
	)
}

func buildReplaySubsystem(ctx context.Context, cfg config.Config, database *postgres.Database, logger *slog.Logger) (*replay.Service, error) {
	if !cfg.Replay.Enabled {
		return nil, nil
	}
	policy, policyError := replay.NewPolicy(cfg.Replay.AllowedHosts, cfg.Replay.AllowPrivate)
	if policyError != nil {
		logger.Error("replay policy initialization failed", "error", policyError)
		return nil, policyError
	}
	executor, executorError := replay.NewExecutor(policy, cfg.Replay.Timeout, cfg.Replay.MaxResponseBytes, cfg.Replay.MaxConcurrent)
	if executorError != nil {
		logger.Error("replay executor initialization failed", "error", executorError)
		return nil, executorError
	}
	cleanupCtx, cancelCleanup := context.WithTimeout(ctx, 5*time.Second)
	if _, cleanupError := database.FailStaleReplayExecutions(cleanupCtx, cfg.Replay.Timeout); cleanupError != nil {
		logger.Warn("stale replay execution cleanup failed", "error", cleanupError)
	}
	cancelCleanup()
	return replay.NewService(database, executor), nil
}

func buildOTLPReceiver(cfg config.Config, pipe *pipeline.Pipeline, metricsPipe *pipeline.MetricsPipeline, selfMetrics *metrics.Registry, database *postgres.Database) *ingestotlp.Receiver {
	var authenticateOTLP func(context.Context, string) (auth.Principal, error)
	if cfg.Auth.BootstrapAPIKey != "" {
		authenticateOTLP = database.AuthenticateAPIKey
	}
	return ingestotlp.NewReceiver(cfg.Ingestion, pipe, metricsPipe, selfMetrics, authenticateOTLP)
}

func buildReadinessCheck(pipe *pipeline.Pipeline, otlp *ingestotlp.Server, database *postgres.Database) func(context.Context) error {
	return func(ctx context.Context) error {
		if !pipe.Ready() || !otlp.Ready() {
			return errors.New("ingestion runtime is not ready")
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
	if cfg.Auth.BootstrapAPIKey == "" {
		logger.Warn("OTLP ingest authentication is disabled")
	}
}
