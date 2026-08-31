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
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"github.com/vanquan3008/tracepilot/internal/replay"
	"github.com/vanquan3008/tracepilot/internal/retention"
	"github.com/vanquan3008/tracepilot/internal/storage/migrations"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi"
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
	var authenticateOTLP func(context.Context, string) (auth.Principal, error)
	if configuration.Auth.BootstrapAPIKey != "" {
		authenticateOTLP = database.AuthenticateAPIKey
	}
	otlpReceiver := ingestotlp.NewReceiver(configuration.Ingestion, ingestionPipeline, selfMetrics, authenticateOTLP)
	otlpServer := ingestotlp.NewServer(configuration, otlpReceiver)
	var replayService *replay.Service
	if replayExecutor != nil {
		replayService = replay.NewService(database, replayExecutor)
	}
	managementServer := httpapi.New(configuration, logger, func(readinessContext context.Context) error {
		if !ingestionPipeline.Ready() || !otlpServer.Ready() {
			return errors.New("ingestion runtime is not ready")
		}
		return database.Ready(readinessContext)
	}, selfMetrics, database, replayService)

	group, runtimeContext := errgroup.WithContext(applicationContext)

	group.Go(func() error {
		return managementServer.Run(runtimeContext)
	})
	group.Go(func() error {
		return otlpServer.Run(runtimeContext)
	})
	if configuration.Retention.MaxAge > 0 {
		group.Go(func() error {
			return retention.Run(runtimeContext, logger, database, selfMetrics, configuration.Retention.MaxAge, configuration.Retention.CheckInterval, configuration.Retention.BatchSize)
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
