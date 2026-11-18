package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/vanquan3008/tracepilot/internal/application/intelligence"
	"github.com/vanquan3008/tracepilot/internal/application/live"
	"github.com/vanquan3008/tracepilot/internal/application/notification"
	"github.com/vanquan3008/tracepilot/internal/application/partition"
	"github.com/vanquan3008/tracepilot/internal/application/retention"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/cache"
	"github.com/vanquan3008/tracepilot/internal/config"
	ingestotlp "github.com/vanquan3008/tracepilot/internal/ingest/otlp"
	"github.com/vanquan3008/tracepilot/internal/observability/logging"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"github.com/vanquan3008/tracepilot/internal/storage/hybrid"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi"
)

func runServer(arguments []string) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(arguments); err != nil {
		return 2
	}

	if flags.NArg() != 0 {
		slog.Error("serve does not accept positional arguments")
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		return 2
	}

	logger := logging.NewLogger(os.Stdout, &logging.Options{
		Level:          cfg.LogLevel,
		ServiceName:    "tracepilot",
		ServiceVersion: version,
	})
	slog.SetDefault(logger)

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

	store, err := hybrid.Open(appCtx, cfg)
	if err != nil {
		logger.Error("storage initialization failed", "error", err)
		return 1
	}
	defer store.Close()
	database := store.Database

	if err := partition.Ensure(appCtx, database, cfg.Database.PartitionWindowDays); err != nil {
		logger.Error("telemetry partition initialization failed", "error", err)
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

	ingestionPipeline := buildIngestionPipeline(cfg, eventWriter{store: store}, scopes, selfMetrics)
	metricsPipeline := buildMetricsPipeline(cfg, database, scopes, selfMetrics)

	// ---------------------------------------------------------
	// 5. Transport servers & Caching
	// ---------------------------------------------------------

	cacheStore, err := cache.New(cfg.Cache, logger)
	if err != nil {
		logger.Error("cache initialization failed", "error", err)
		return 1
	}
	defer cacheStore.Close()

	tokenCache := auth.NewCachedTokenSource(database, cfg.Cache.EffectiveTokenTTL(), cfg.Cache.EffectiveTokenLimit()).
		WithByteCache(cacheStore).
		WithObserver(selfMetrics)
	otlpReceiver := buildOTLPReceiver(cfg, ingestionPipeline, metricsPipeline, selfMetrics, tokenCache, scopes)
	otlpServer := ingestotlp.NewServer(cfg, otlpReceiver)

	streamHub := live.NewHub()
	ingestionPipeline.SetOnPersist(streamHub.BroadcastEvents)

	managementServer := httpapi.New(
		cfg,
		logger,
		buildReadinessCheck(store, ingestionPipeline, metricsPipeline, otlpServer),
		selfMetrics,
		store,
		replayService,
	).WithTokenInvalidator(tokenCache).WithStreamHub(streamHub)

	// ---------------------------------------------------------
	// 6. Runtime components
	// ---------------------------------------------------------

	components := []component{
		{"management-server", true, managementServer.Run},
		{"otlp-server", true, otlpServer.Run},
		{"intelligence", false, func(ctx context.Context) error {
			return intelligence.Run(ctx, logger, buildObservabilityDetectionFunc(scopes, database, selfMetrics), intelligence.DefaultInterval)
		}},
		{"partition-maintenance", false, func(ctx context.Context) error {
			return partition.Run(ctx, logger, database, cfg.Database.PartitionWindowDays, cfg.Database.PartitionCheckInterval)
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
		components = append(components, component{"webhook-notification", false, webhookService.Run})
	}

	policy := retention.Policy{
		Telemetry:  cfg.Retention.MaxAge,
		Incidents:  cfg.Retention.IncidentMaxAge,
		Audit:      cfg.Retention.AuditMaxAge,
		Deliveries: cfg.Retention.DeliveryMaxAge,
	}
	if policy.Enabled() {
		components = append(components, component{"retention", false, func(ctx context.Context) error {
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

	logStartup(logger, cfg, version)
	logger.Info("server started")

	// Wait until an OS termination signal is received, or a component fails or exits unexpectedly.
	serveError := superviseComponents(appCtx, logger, components)

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
	shutdownError := drainPipelines(shutdownCtx, logger, ingestionPipeline, metricsPipeline)

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
