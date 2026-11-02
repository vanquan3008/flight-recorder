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

	"github.com/vanquan3008/tracepilot/internal/application/intelligence"
	"github.com/vanquan3008/tracepilot/internal/application/notification"
	"github.com/vanquan3008/tracepilot/internal/application/retention"
	"github.com/vanquan3008/tracepilot/internal/config"
	ingestotlp "github.com/vanquan3008/tracepilot/internal/ingest/otlp"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"github.com/vanquan3008/tracepilot/internal/storage/migrations"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi"
)

func runServer(arguments []string) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	if err := flags.Parse(arguments); err != nil {
		return 2
	}

	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "serve does not accept positional arguments")
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", err)
		return 2
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	if err := ensureJWTSecret(&cfg, logger); err != nil {
		logger.Error("JWT secret generation failed", "error", err)
		return 1
	}

	// Application lifecycle context.
	appCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	// ---------------------------------------------------------
	// 1. Database initialization
	// ---------------------------------------------------------

	logger.Info("Checking database schema")

	if err := migrations.EnsureCurrent(cfg.Database.URL, cfg.Database.MigrationsPath); err != nil {
		logger.Error("database schema check failed", "error", err)
		return 1
	}

	database, err := postgres.OpenWithWebhookEncryptionKey(appCtx, cfg.Database, cfg.Notifications.EncryptionKey)
	if err != nil {
		logger.Error("database initialization failed", "error", err)
		return 1
	}
	defer database.Close()

	if err := ensureTelemetryPartitions(appCtx, database); err != nil {
		logger.Error("telemetry partition initialization failed", "error", err)
		return 1
	}

	if err := database.EnsureIngestionScope(appCtx, cfg.Ingestion); err != nil {
		logger.Error("ingestion scope initialization failed", "error", err)
		return 1
	}

	// Do not continue startup if the process is already shutting down.
	if err := appCtx.Err(); err != nil {
		logger.Info("startup interrupted", "error", err)
		return 0
	}

	// ---------------------------------------------------------
	// 2. Application services
	// ---------------------------------------------------------

	replayService, err := buildReplaySubsystem(appCtx, cfg, database, logger)
	if err != nil {
		logger.Error("replay subsystem initialization failed", "error", err)
		return 1
	}

	// ---------------------------------------------------------
	// 3. Observability
	// ---------------------------------------------------------

	selfMetrics := metrics.New(version, commit, buildTime)
	selfMetrics.WatchPool(database.PoolSnapshot)

	// ---------------------------------------------------------
	// 4. Processing pipelines
	// ---------------------------------------------------------

	// Projects and environments are data, so the scope telemetry is stored under is looked up per
	// request; the configured project keeps working without a lookup.
	scopes := newScopeCatalog(cfg.Ingestion, database)

	ingestionPipeline := buildIngestionPipeline(cfg, database, scopes, selfMetrics)
	metricsPipeline := buildMetricsPipeline(cfg, database, scopes, selfMetrics)

	// ---------------------------------------------------------
	// 5. Transport servers
	// ---------------------------------------------------------

	otlpReceiver := buildOTLPReceiver(cfg, ingestionPipeline, metricsPipeline, selfMetrics, database, scopes)
	otlpServer := ingestotlp.NewServer(cfg, otlpReceiver)

	managementServer := httpapi.New(
		cfg,
		logger,
		buildReadinessCheck(database, ingestionPipeline, metricsPipeline, otlpServer),
		selfMetrics,
		database,
		replayService,
	)

	// ---------------------------------------------------------
	// 6. Runtime components
	// ---------------------------------------------------------

	components := []component{
		{"management-server", managementServer.Run},
		{"otlp-server", otlpServer.Run},
		{"intelligence", func(ctx context.Context) error {
			return intelligence.Run(ctx, logger, buildObservabilityDetectionFunc(scopes, database, selfMetrics), intelligence.DefaultInterval)
		}},
		{"partition-maintenance", func(ctx context.Context) error {
			return runPartitionMaintenance(ctx, logger, database)
		}},
	}

	// Optional outbound notifications.
	if cfg.Notifications.Enabled {
		webhookService := notification.NewService(database, logger, notification.Config{
			PollInterval: cfg.Notifications.PollInterval,
			Workers:      cfg.Notifications.Workers,
			Timeout:      cfg.Notifications.RequestTimeout,
			Observer:     selfMetrics,
		})
		components = append(components, component{"webhook-notification", webhookService.Run})
	}

	policy := retention.Policy{
		Telemetry:  cfg.Retention.MaxAge,
		Incidents:  cfg.Retention.IncidentMaxAge,
		Audit:      cfg.Retention.AuditMaxAge,
		Deliveries: cfg.Retention.DeliveryMaxAge,
	}
	if policy.Enabled() {
		components = append(components, component{"retention", func(ctx context.Context) error {
			return retention.Run(ctx, logger, database, selfMetrics, policy, cfg.Retention.CheckInterval, cfg.Retention.BatchSize)
		}})
	}

	// ---------------------------------------------------------
	// 7. Start application
	// ---------------------------------------------------------

	if err := appCtx.Err(); err != nil {
		logger.Info("startup interrupted", "error", err)
		return 0
	}

	logger.Info("starting processing pipelines")
	ingestionPipeline.Start()
	metricsPipeline.Start()

	logStartup(logger, cfg)
	logger.Info("server started")

	// Wait until an OS termination signal is received, or a component fails or exits unexpectedly.
	serveError := superviseComponents(appCtx, components)

	// ---------------------------------------------------------
	// 8. Graceful shutdown
	// ---------------------------------------------------------

	if appCtx.Err() != nil {
		logger.Info("shutdown signal received")
	} else if serveError != nil {
		logger.Error("runtime component failed", "error", serveError)
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancelShutdown()

	logger.Info("shutting down processing pipelines", "timeout", cfg.ShutdownTimeout)
	shutdownError := drainPipelines(shutdownCtx, ingestionPipeline, metricsPipeline)

	// Expected cancellation caused by an OS signal is not a failure; real component errors are kept.
	if appCtx.Err() != nil && errors.Is(serveError, context.Canceled) {
		serveError = nil
	}

	if runtimeError := errors.Join(serveError, shutdownError); runtimeError != nil {
		logger.Error("server stopped with errors", "error", runtimeError)
		return 1
	}

	logger.Info("server stopped successfully")
	return 0
}
