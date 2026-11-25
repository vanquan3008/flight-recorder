// Command aidemo emits a real OpenTelemetry trace for a disposable refund-policy demo.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	serviceName           = "tracepilot-ai-demo"
	demoAddress           = "127.0.0.1:8090"
	demoCollectorEndpoint = "http://127.0.0.1:14318/v1/traces"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		logger.Error("cannot read backend/.env")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, logger); err != nil {
		logger.Error("AI demo stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	model, err := configuredOpenAIModel(os.Getenv("TRACEPILOT_AI_DEMO_OPENAI_MODEL"), os.Getenv("TRACEPILOT_AI_DEMO_OPENAI_API_KEY"))
	if err != nil {
		return err
	}
	capture, err := contentCapture(os.Getenv("TRACEPILOT_AI_DEMO_CAPTURE_CONTENT"), os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"))
	if err != nil {
		return err
	}
	startup, cancelStartup := context.WithTimeout(ctx, 5*time.Second)
	pool, err := openDemoDatabase(startup, os.Getenv("TRACEPILOT_AI_DEMO_DATABASE_URL"))
	cancelStartup()
	if err != nil {
		return err
	}
	defer pool.Close()

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return fmt.Errorf("initialize OTLP trace exporter: %w", err)
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", serviceName))),
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(time.Second)),
	)
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := provider.Shutdown(shutdown); err != nil {
			logger.Warn("trace exporter shutdown failed", "error", err)
		}
	}()

	logs, err := configuredLogExporter(logger)
	if err != nil {
		return err
	}
	if logs != nil {
		defer func() {
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := logs.shutdown(shutdown); err != nil {
				logger.Warn("OTLP log export drain timed out")
			}
		}()
	}
	app := demo{tracer: provider.Tracer(serviceName), logger: logger, lookup: policyLookup(pool), capture: capture, model: model, logs: logs}
	server := &http.Server{
		Addr: demoAddress, Handler: app.routes(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second,
	}
	stopped := make(chan error, 1)
	go func() { stopped <- server.ListenAndServe() }()
	logger.Info("AI demo starting", "address", demoAddress)
	select {
	case err := <-stopped:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}

func contentCapture(value, endpoint string) (bool, error) {
	if value == "" {
		return false, nil
	}
	capture, err := strconv.ParseBool(value)
	if err != nil {
		return false, errors.New("TRACEPILOT_AI_DEMO_CAPTURE_CONTENT must be a boolean")
	}
	if capture && endpoint != demoCollectorEndpoint {
		return false, fmt.Errorf("demo content capture requires OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=%s", demoCollectorEndpoint)
	}
	return capture, nil
}

func openDemoDatabase(ctx context.Context, url string) (*pgxpool.Pool, error) {
	if url == "" {
		return nil, errors.New("set TRACEPILOT_AI_DEMO_DATABASE_URL to a disposable PostgreSQL database")
	}
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errors.New("invalid demo PostgreSQL connection settings")
	}
	config.MaxConns = 2
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		// Each connection gets its own fixture. No persistent schema is required.
		_, err := conn.Exec(ctx, `
CREATE TEMP TABLE refund_policies (version integer PRIMARY KEY, refund_days integer NOT NULL);
INSERT INTO pg_temp.refund_policies (version, refund_days) VALUES (1, 14), (2, 30);`)
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.New("cannot open demo PostgreSQL pool")
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errors.New("cannot connect to demo PostgreSQL or seed its temporary fixture")
	}
	return pool, nil
}

func policyLookup(pool *pgxpool.Pool) func(context.Context, int32) (refundPolicy, error) {
	return func(ctx context.Context, version int32) (refundPolicy, error) {
		var policy refundPolicy
		err := pool.QueryRow(ctx, `
SELECT version, refund_days FROM pg_temp.refund_policies
WHERE ($1::integer = 0 OR version = $1)
ORDER BY version DESC LIMIT 1`, version).Scan(&policy.version, &policy.days)
		return policy, err
	}
}
