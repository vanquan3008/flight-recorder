package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	ingestotlp "github.com/vanquan3008/tracepilot/internal/ingest/otlp"
	"github.com/vanquan3008/tracepilot/internal/ingest/pipeline"
	ingestsyslog "github.com/vanquan3008/tracepilot/internal/ingest/syslog"
	"github.com/vanquan3008/tracepilot/internal/notification"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"github.com/vanquan3008/tracepilot/internal/replay"
	"github.com/vanquan3008/tracepilot/internal/retention"
	securityingest "github.com/vanquan3008/tracepilot/internal/security"
	"github.com/vanquan3008/tracepilot/internal/security/detect"
	"github.com/vanquan3008/tracepilot/internal/storage/migrations"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi"
)

func runServer(arguments []string) int {
	flags := flag.NewFlagSet("Serve", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	configFile := flags.String("config", "", "Path to a versioned YAML configuration file")
	if parseError := flags.Parse(arguments); parseError != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "serve does not accept positional arguments")
		return 2
	}
	cfg, loadError := config.Load(*configFile)
	if loadError != nil {
		fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", loadError)
		return 2
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	appCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	if migrationError := migrations.EnsureCurrent(cfg.Database.URL, cfg.Database.MigrationsPath); migrationError != nil {
		logger.Error("database schema check failed", "error", migrationError)
		return 1
	}

	// The database adapter owns encrypted webhook secrets; receive the key as an
	// explicit dependency without mutating the validated application config.
	database, databaseError := postgres.OpenWithWebhookEncryptionKey(appCtx, cfg.Database, cfg.Notifications.EncryptionKey)
	if databaseError != nil {
		logger.Error("database initialization failed", "error", databaseError)
		return 1
	}
	defer database.Close()

	if scopeError := database.EnsureIngestionScope(appCtx, cfg.Ingestion); scopeError != nil {
		logger.Error("ingestion scope initialization failed", "error", scopeError)
		return 1
	}
	if authError := database.EnsureBootstrapAPIKey(appCtx, cfg.Ingestion.ProjectID, cfg.Auth.BootstrapAPIKey); authError != nil {
		logger.Error("bootstrap API key initialization failed", "error", authError)
		return 1
	}

	replayService, replayError := buildReplaySubsystem(appCtx, cfg, database, logger)
	if replayError != nil {
		return 1
	}

	selfMetrics := metrics.New(version, commit, buildTime)
	ingestionPipeline := buildIngestionPipeline(cfg, database, selfMetrics)
	ingestionPipeline.Start()

	metricsPipeline := buildMetricsPipeline(cfg, database, selfMetrics)
	metricsPipeline.Start()

	logStartup(logger, cfg)

	otlpReceiver := buildOTLPReceiver(cfg, ingestionPipeline, metricsPipeline, selfMetrics, database)
	otlpServer := ingestotlp.NewServer(cfg, otlpReceiver)
	syslogServer := buildSyslogServer(cfg, ingestionPipeline, selfMetrics, logger)

	managementServer := httpapi.New(cfg, logger, buildReadinessCheck(ingestionPipeline, otlpServer, syslogServer, database), selfMetrics, database, replayService)
	var webhookService *notification.Service
	if cfg.Notifications.Enabled {
		webhookService = notification.NewService(database, logger, notification.Config{
			PollInterval: cfg.Notifications.PollInterval,
			Workers:      cfg.Notifications.Workers,
			Timeout:      cfg.Notifications.RequestTimeout,
			Observer:     selfMetrics,
		})
	}

	group, runtimeCtx := errgroup.WithContext(appCtx)
	group.Go(func() error { return managementServer.Run(runtimeCtx) })
	group.Go(func() error { return otlpServer.Run(runtimeCtx) })
	if webhookService != nil {
		group.Go(func() error { return webhookService.Run(runtimeCtx) })
	}

	if syslogServer != nil {
		group.Go(func() error { return syslogServer.Run(runtimeCtx) })
		group.Go(func() error {
			return detect.Run(runtimeCtx, logger, buildDetectionFunc(cfg, database, selfMetrics), cfg.Security.DetectionInterval)
		})
	}

	if cfg.Retention.MaxAge > 0 {
		group.Go(func() error {
			return retention.Run(runtimeCtx, logger, database, selfMetrics, cfg.Retention.MaxAge, cfg.Retention.CheckInterval, cfg.Retention.BatchSize)
		})
	}

	serveError := group.Wait()

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	pipelineError := ingestionPipeline.Shutdown(shutdownCtx)
	metricsShutdownError := metricsPipeline.Shutdown(shutdownCtx)
	cancelShutdown()

	if runtimeError := errors.Join(serveError, pipelineError, metricsShutdownError); runtimeError != nil {
		logger.Error("server stopped", "error", runtimeError)
		return 1
	}

	logger.Info("server stopped")
	return 0
}

// buildMetricsPipeline creates the best-effort OTLP metrics ingest pipeline.
func buildMetricsPipeline(cfg config.Config, database *postgres.Database, selfMetrics *metrics.Registry) *pipeline.MetricsPipeline {
	return pipeline.NewMetricsPipeline(
		pipeline.MetricsPipelineConfig{
			QueueSize: cfg.Ingestion.QueueSize,
			BatchSize: cfg.Ingestion.BatchSize,
			Workers:   cfg.Ingestion.Workers,
			Metrics:   selfMetrics,
		},
		func(ctx context.Context, points []telemetry.MetricDataPoint) error {
			return database.StoreMetrics(ctx, cfg.Ingestion, points)
		},
	)
}

// buildIngestionPipeline wires the pipeline with the security-processing store
// function. Security normalization is enabled explicitly so the default
// observability-only deployment does not classify ordinary application logs as
// security events.
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
			if !cfg.Security.Enabled {
				return database.StoreEvents(storeCtx, cfg.Ingestion, events)
			}
			return database.StoreEvents(storeCtx, cfg.Ingestion, securityingest.Process(events))
		},
		selfMetrics,
	)
}

// buildReplaySubsystem initialises the replay executor and service when replay
// is enabled, cleans up stale executions, and returns nil when disabled.
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

// buildOTLPReceiver creates the OTLP receiver with optional API key auth and metrics pipeline.
func buildOTLPReceiver(cfg config.Config, pipe *pipeline.Pipeline, metricsPipe *pipeline.MetricsPipeline, selfMetrics *metrics.Registry, database *postgres.Database) *ingestotlp.Receiver {
	var authenticateOTLP func(context.Context, string) (auth.Principal, error)
	if cfg.Auth.BootstrapAPIKey != "" {
		authenticateOTLP = database.AuthenticateAPIKey
	}
	return ingestotlp.NewReceiver(cfg.Ingestion, pipe, metricsPipe, selfMetrics, authenticateOTLP)
}

// buildSyslogServer returns nil when security is disabled so callers can check
// once rather than gate every start/ready call.
func buildSyslogServer(cfg config.Config, pipe *pipeline.Pipeline, selfMetrics *metrics.Registry, logger *slog.Logger) *ingestsyslog.Server {
	if !cfg.Security.Enabled {
		return nil
	}
	return ingestsyslog.New(cfg.Security, cfg.Ingestion, pipe, selfMetrics, logger, cfg.TLS.CertificateFile, cfg.TLS.PrivateKeyFile)
}

// buildReadinessCheck returns a closure that gates readiness on the pipeline,
// OTLP server, optional syslog server, and the database.
func buildReadinessCheck(pipe *pipeline.Pipeline, otlp *ingestotlp.Server, syslog *ingestsyslog.Server, database *postgres.Database) func(context.Context) error {
	return func(ctx context.Context) error {
		if !pipe.Ready() || !otlp.Ready() || (syslog != nil && !syslog.Ready()) {
			return errors.New("ingestion runtime is not ready")
		}
		return database.Ready(ctx)
	}
}

// buildDetectionFunc wraps EvaluateSecurityRules with metrics observation so
// the detect.Run loop stays free of observability concerns.
func buildDetectionFunc(cfg config.Config, database *postgres.Database, selfMetrics *metrics.Registry) func(context.Context) (int, error) {
	return func(ctx context.Context) (int, error) {
		startedAt := time.Now()
		count, evaluateError := database.EvaluateSecurityRules(ctx, cfg.Ingestion)
		selfMetrics.ObserveSecurityDetection(time.Since(startedAt), count, evaluateError != nil)
		if evaluateError == nil {
			if lag, age, healthError := database.SecurityDetectionHealth(ctx, cfg.Ingestion); healthError == nil {
				selfMetrics.SetSecurityDetectionLag(lag)
				selfMetrics.SetSecurityDetectionCheckpointAge(age)
			}
		}
		return count, evaluateError
	}
}

// logStartup emits structured startup information and any configuration warnings.
func logStartup(logger *slog.Logger, cfg config.Config) {
	logger.Info("Starting TracePilot",
		"version", version,
		"http_address", cfg.HTTP.Address,
		"otlp_grpc_address", cfg.Ingestion.GRPCAddress,
		"otlp_http_address", cfg.Ingestion.HTTPAddress,
	)
	if cfg.Security.Enabled {
		logger.Info("security analytics enabled",
			"syslog_address", cfg.Security.SyslogAddress,
			"syslog_tls", cfg.Security.SyslogTLS,
		)
	}
	if cfg.TLS.CertificateFile == "" {
		logger.Warn("OTLP listeners are plaintext; use TLS or terminate TLS before TracePilot")
	}
	if cfg.Auth.BootstrapAPIKey == "" {
		logger.Warn("OTLP ingest authentication is disabled")
	}
}
