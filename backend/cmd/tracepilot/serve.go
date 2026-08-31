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

	"github.com/vanquan3008/tracepilot/internal/config"
	ingestotlp "github.com/vanquan3008/tracepilot/internal/ingest/otlp"
	"github.com/vanquan3008/tracepilot/internal/management"
	"github.com/vanquan3008/tracepilot/internal/metrics"
	"github.com/vanquan3008/tracepilot/internal/migrations"
	"github.com/vanquan3008/tracepilot/internal/pipeline"
	"github.com/vanquan3008/tracepilot/internal/postgres"
	"github.com/vanquan3008/tracepilot/internal/replay"
	"github.com/vanquan3008/tracepilot/internal/telemetry"
)

func runServer(arguments []string) int {
	flags := flag.NewFlagSet("Serve", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	configFile := flags.String("config", "", "path to a versioned YAML configuration file")
	if parseError := flags.Parse(arguments); parseError != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "serve does not accept positional arguments")
		return 2
	}

	configuration, loadError := config.Load(*configFile)
	if loadError != nil {
		fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", loadError)
		return 2
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: configuration.LogLevel}))
	applicationContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	if migrationError := migrations.EnsureCurrent(configuration.Database.URL, configuration.Database.MigrationsPath); migrationError != nil {
		logger.Error("database schema check failed", "error", migrationError)
		return 1
	}

	database, databaseError := postgres.Open(applicationContext, configuration.Database)
	if databaseError != nil {
		logger.Error("database initialization failed", "error", databaseError)
		return 1
	}
	defer database.Close()
	if scopeError := database.EnsureIngestionScope(applicationContext, configuration.Ingestion); scopeError != nil {
		logger.Error("ingestion scope initialization failed", "error", scopeError)
		return 1
	}
	if authError := database.EnsureBootstrapAPIKey(applicationContext, configuration.Ingestion.ProjectID, configuration.Auth.BootstrapAPIKey); authError != nil {
		logger.Error("bootstrap API key initialization failed", "error", authError)
		return 1
	}
	var replayExecutor *replay.Executor
	if configuration.Replay.Enabled {
		policy, policyError := replay.NewPolicy(configuration.Replay.AllowedHosts, configuration.Replay.AllowPrivate)
		if policyError != nil {
			logger.Error("replay policy initialization failed", "error", policyError)
			return 1
		}
		replayExecutor, policyError = replay.NewExecutor(policy, configuration.Replay.Timeout, configuration.Replay.MaxResponseBytes, configuration.Replay.MaxConcurrent)
		if policyError != nil {
			logger.Error("replay executor initialization failed", "error", policyError)
			return 1
		}
		cleanupContext, cancelCleanup := context.WithTimeout(applicationContext, 5*time.Second)
		if _, cleanupError := database.FailStaleReplayExecutions(cleanupContext, configuration.Replay.Timeout); cleanupError != nil {
			logger.Warn("stale replay execution cleanup failed", "error", cleanupError)
		}
		cancelCleanup()
	}

	selfMetrics := metrics.New(version, commit, buildTime)
	ingestionPipeline := pipeline.New(pipeline.Config{
		QueueSize:      configuration.Ingestion.QueueSize,
		QueueMaxBytes:  configuration.Ingestion.QueueMaxBytes,
		BatchSize:      configuration.Ingestion.BatchSize,
		Workers:        configuration.Ingestion.Workers,
		EnqueueTimeout: configuration.Ingestion.EnqueueTimeout,
		BatchTimeout:   configuration.Ingestion.BatchTimeout,
		FlushInterval:  configuration.Ingestion.FlushInterval,
	},
		func(storeContext context.Context, events []telemetry.Event) error {
			return database.StoreEvents(storeContext, configuration.Ingestion, events)
		},
		selfMetrics,
	)
	ingestionPipeline.Start()

	logger.Info("Starting TracePilot",
		"version", version,
		"http_address", configuration.HTTP.Address,
		"otlp_grpc_address", configuration.Ingestion.GRPCAddress,
		"otlp_http_address", configuration.Ingestion.HTTPAddress,
	)
	if configuration.TLS.CertificateFile == "" {
		logger.Warn("OTLP listeners are plaintext; use TLS or terminate TLS before TracePilot")
	}
	if configuration.Auth.BootstrapAPIKey == "" {
		logger.Warn("OTLP ingest authentication is disabled")
	}
	var authenticateOTLP func(context.Context, string) (postgres.Principal, error)
	if configuration.Auth.BootstrapAPIKey != "" {
		authenticateOTLP = database.AuthenticateAPIKey
	}
	otlpReceiver := ingestotlp.NewReceiver(configuration.Ingestion, ingestionPipeline, selfMetrics, authenticateOTLP)
	otlpServer := ingestotlp.NewServer(configuration, otlpReceiver)
	managementServer := management.New(configuration, logger, func(readinessContext context.Context) error {
		if !ingestionPipeline.Ready() || !otlpServer.Ready() {
			return errors.New("ingestion runtime is not ready")
		}
		return database.Ready(readinessContext)
	}, selfMetrics, database, replayExecutor)

	group, runtimeContext := errgroup.WithContext(applicationContext)

	group.Go(func() error {
		return managementServer.Run(runtimeContext)
	})
	group.Go(func() error {
		return otlpServer.Run(runtimeContext)
	})
	if configuration.Retention.MaxAge > 0 {
		group.Go(func() error {
			return runRetention(runtimeContext, logger, database, selfMetrics, configuration.Retention.MaxAge, configuration.Retention.CheckInterval, configuration.Retention.BatchSize)
		})
	}

	serveError := group.Wait()

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), configuration.ShutdownTimeout)
	pipelineError := ingestionPipeline.Shutdown(shutdownContext)
	cancelShutdown()

	if runtimeError := errors.Join(serveError, pipelineError); runtimeError != nil {
		logger.Error("server stopped", "error", runtimeError)
		return 1
	}

	logger.Info("server stopped")
	return 0
}

func runRetention(ctx context.Context, logger *slog.Logger, database *postgres.Database, selfMetrics *metrics.Registry, maxAge, interval time.Duration, batchSize int) error {
	prune := func() {
		startedAt := time.Now()
		pruneContext, cancelPrune := context.WithTimeout(ctx, 30*time.Second)
		before := time.Now().UTC().Add(-maxAge)
		var deleted int64
		var pruneError error
		deleted, pruneError = pruneExpiredEvents(pruneContext, before, batchSize, database.DeleteEventsBefore)
		if pruneError == nil {
			if backlog, backlogError := database.CountEventsBefore(pruneContext, before); backlogError != nil {
				logger.Warn("retention backlog metric failed", "error", backlogError)
			} else {
				selfMetrics.SetRetentionBacklog(backlog)
			}
		}
		cancelPrune()
		selfMetrics.ObserveRetention(time.Since(startedAt), deleted, pruneError != nil)
		if pruneError != nil {
			logger.Error("retention cleanup failed", "error", pruneError)
			return
		}
		if deleted > 0 {
			logger.Info("retention cleanup completed", "deleted_events", deleted)
		}
	}

	prune()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			prune()
		}
	}
}

func pruneExpiredEvents(ctx context.Context, before time.Time, batchSize int, deleteBatch func(context.Context, time.Time, int) (int64, error)) (int64, error) {
	if batchSize <= 0 {
		return 0, errors.New("retention batch size must be positive")
	}
	var deleted int64
	for {
		if ctx.Err() != nil {
			return deleted, ctx.Err()
		}
		batchDeleted, deleteError := deleteBatch(ctx, before, batchSize)
		deleted += batchDeleted
		if deleteError != nil {
			return deleted, deleteError
		}
		if batchDeleted < int64(batchSize) {
			return deleted, nil
		}
	}
}
