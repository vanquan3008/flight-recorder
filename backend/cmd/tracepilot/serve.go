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

	"golang.org/x/sync/errgroup"

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
	database, databaseError := postgres.OpenWithWebhookEncryptionKey(appCtx, cfg.Database, cfg.Notifications.EncryptionKey)
	if databaseError != nil {
		logger.Error("database initialization failed", "error", databaseError)
		return 1
	}
	defer database.Close()

	if partitionError := ensureTelemetryPartitions(appCtx, database); partitionError != nil {
		logger.Error("telemetry partition initialization failed", "error", partitionError)
		return 1
	}
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
	selfMetrics.WatchPool(database.PoolSnapshot)
	ingestionPipeline := buildIngestionPipeline(cfg, database, selfMetrics)
	ingestionPipeline.Start()

	metricsPipeline := buildMetricsPipeline(cfg, database, selfMetrics)
	metricsPipeline.Start()

	logStartup(logger, cfg)

	otlpReceiver := buildOTLPReceiver(cfg, ingestionPipeline, metricsPipeline, selfMetrics, database)
	otlpServer := ingestotlp.NewServer(cfg, otlpReceiver)

	managementServer := httpapi.New(cfg, logger, buildReadinessCheck(ingestionPipeline, otlpServer, database), selfMetrics, database, replayService)
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
	group.Go(func() error {
		return intelligence.Run(runtimeCtx, logger, buildObservabilityDetectionFunc(cfg, database, selfMetrics), intelligence.DefaultInterval)
	})
	if webhookService != nil {
		group.Go(func() error { return webhookService.Run(runtimeCtx) })
	}
	group.Go(func() error { return runPartitionMaintenance(runtimeCtx, logger, database) })

	policy := retention.Policy{
		Telemetry: cfg.Retention.MaxAge, Incidents: cfg.Retention.IncidentMaxAge,
		Audit: cfg.Retention.AuditMaxAge, Deliveries: cfg.Retention.DeliveryMaxAge,
	}
	if policy.Enabled() {
		group.Go(func() error {
			return retention.Run(runtimeCtx, logger, database, selfMetrics, policy, cfg.Retention.CheckInterval, cfg.Retention.BatchSize)
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
