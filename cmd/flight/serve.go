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

	"github.com/flight-observability/flight/internal/config"
	"github.com/flight-observability/flight/internal/httpapi"
	ingestotlp "github.com/flight-observability/flight/internal/ingest/otlp"
	"github.com/flight-observability/flight/internal/metrics"
	"github.com/flight-observability/flight/internal/pipeline"
	"github.com/flight-observability/flight/internal/postgres"
	"github.com/flight-observability/flight/internal/telemetry"
)

func runServer(arguments []string) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
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

	database, databaseError := postgres.Open(applicationContext, configuration.Database)
	if databaseError != nil {
		logger.Error("database initialization failed", "error", databaseError)
		return 1
	}
	defer database.Close()

	metricRegistry := metrics.New(version, commit, buildTime)
	ingestionPipeline := pipeline.New(
		configuration.Ingestion.QueueSize,
		configuration.Ingestion.QueueMaxBytes,
		configuration.Ingestion.BatchSize,
		configuration.Ingestion.Workers,
		configuration.Ingestion.EnqueueTimeout,
		configuration.Ingestion.BatchTimeout,
		configuration.Ingestion.FlushInterval,
		func(storeContext context.Context, events []telemetry.Event) error {
			return database.StoreEvents(storeContext, configuration.Ingestion, events)
		},
		metricRegistry,
	)
	ingestionPipeline.Start()

	logger.Info("starting Flight",
		"version", version,
		"http_address", configuration.HTTP.Address,
		"otlp_grpc_address", configuration.Ingestion.GRPCAddress,
		"otlp_http_address", configuration.Ingestion.HTTPAddress,
	)
	otlpReceiver := ingestotlp.NewReceiver(configuration.Ingestion, ingestionPipeline, metricRegistry)
	otlpServer := ingestotlp.NewServer(configuration, otlpReceiver)
	httpServer := httpapi.New(configuration, logger, func(readinessContext context.Context) error {
		if !ingestionPipeline.Ready() || !otlpServer.Ready() {
			return errors.New("ingestion runtime is not ready")
		}
		return database.Ready(readinessContext)
	}, metricRegistry)
	runtimeContext, cancelRuntime := context.WithCancel(applicationContext)
	results := make(chan error, 2)
	go func() { results <- httpServer.Run(runtimeContext) }()
	go func() { results <- otlpServer.Run(runtimeContext) }()

	serveError := <-results
	cancelRuntime()
	serveError = errors.Join(serveError, <-results)
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
