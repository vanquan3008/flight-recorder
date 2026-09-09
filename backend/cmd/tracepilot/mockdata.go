package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	securityingest "github.com/vanquan3008/tracepilot/internal/security"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
)

func runMockData(arguments []string) int {
	flags := flag.NewFlagSet("mockdata", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	configFile := flags.String("config", "", "path to a versioned YAML configuration file")
	if parseError := flags.Parse(arguments); parseError != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "mockdata does not accept positional arguments")
		return 2
	}

	configuration, loadError := config.Load(*configFile)
	if loadError != nil {
		fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", loadError)
		return 2
	}
	if configuration.Production {
		fmt.Fprintln(os.Stderr, "mockdata is disabled when TRACEPILOT_PRODUCTION=true")
		return 2
	}

	applicationContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, openError := postgres.Open(applicationContext, configuration.Database)
	if openError != nil {
		fmt.Fprintf(os.Stderr, "database initialization failed: %v\n", openError)
		return 1
	}
	defer database.Close()
	if readinessError := database.Ready(applicationContext); readinessError != nil {
		fmt.Fprintf(os.Stderr, "database is not ready: %v (run `tracepilot migrate up` first)\n", readinessError)
		return 1
	}
	if scopeError := database.EnsureIngestionScope(applicationContext, configuration.Ingestion); scopeError != nil {
		fmt.Fprintf(os.Stderr, "ingestion scope initialization failed: %v\n", scopeError)
		return 1
	}
	if configuration.Auth.BootstrapAPIKey != "" {
		if keyError := database.EnsureBootstrapAPIKey(applicationContext, configuration.Ingestion.ProjectID, configuration.Auth.BootstrapAPIKey); keyError != nil {
			fmt.Fprintf(os.Stderr, "bootstrap API key initialization failed: %v\n", keyError)
			return 1
		}
	}

	// mockEvents intentionally calls securityingest.Process so the syslog/CEF
	// event is normalized before StoreEvents, matching the live path where the
	// pipeline's StoreFunc runs Process before every database write. This keeps
	// mock data consistent with what real ingestion would produce.
	events := mockEvents(configuration.Ingestion)
	if storeError := database.StoreEvents(applicationContext, configuration.Ingestion, events); storeError != nil {
		fmt.Fprintf(os.Stderr, "mock data insert failed: %v\n", storeError)
		return 1
	}
	fmt.Printf("mock data seeded: project=%s environment=%s events=%d (safe to rerun)\n", configuration.Ingestion.ProjectSlug, configuration.Ingestion.EnvironmentName, len(events))
	return 0
}

func mockEvents(ingestion config.IngestionConfig) []telemetry.Event {
	// Anchor mock events to 7 days ago so they always appear in the default
	// time-range view regardless of when the binary is run.
	seed := time.Now().UTC().Truncate(24 * time.Hour).Add(-7 * 24 * time.Hour).Add(12 * time.Hour)
	checkoutTrace := "11111111111111111111111111111111"
	catalogTrace := "22222222222222222222222222222222"
	incidentTrace := "33333333333333333333333333333333"
	attributes := func() map[string]any { return map[string]any{"source": "tracepilot-mockdata"} }
	resourceAttributes := func(service string) map[string]any {
		return map[string]any{"service.name": service, "deployment.environment": ingestion.EnvironmentName}
	}
	span := func(identity, service, traceID, spanID, parent, operation, status string, timestamp time.Time, duration time.Duration, kind int32, httpStatus int) telemetry.Event {
		return telemetry.Event{
			ID: telemetry.DeterministicID(seed, "mock", identity), ProjectID: ingestion.ProjectID, Environment: ingestion.EnvironmentName,
			ServiceName: service, Type: telemetry.EventHTTPServer, Timestamp: timestamp, Duration: duration,
			TraceID: traceID, SpanID: spanID, ParentSpanID: parent, Operation: operation, Status: status, HTTPStatus: httpStatus,
			SpanKind: kind, Signal: telemetry.SignalTrace, Attributes: attributes(), ResourceAttributes: resourceAttributes(service),
			ScopeAttributes: map[string]any{}, RecordAttributes: attributes(),
		}
	}
	log := func(identity, service, traceID, severity, message string, timestamp time.Time) telemetry.Event {
		return telemetry.Event{
			ID: telemetry.DeterministicID(seed, "mock", identity), ProjectID: ingestion.ProjectID, Environment: ingestion.EnvironmentName,
			ServiceName: service, Type: telemetry.EventLog, Timestamp: timestamp, TraceID: traceID, Severity: severity,
			Signal: telemetry.SignalLog, Attributes: attributes(), ResourceAttributes: resourceAttributes(service),
			ScopeAttributes: map[string]any{}, RecordAttributes: attributes(), LogBody: map[string]any{"message": message},
		}
	}
	failedPayment := span("payment-timeout", "payment", checkoutTrace, "aaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbb", "POST /charge", "ERROR", seed.Add(80*time.Millisecond), 900*time.Millisecond, 3, 502)
	failedPayment.Type = telemetry.EventHTTPClient
	failedPayment.ErrorType = "UpstreamTimeout"
	failedPayment.ErrorMessage = "payment provider timed out"
	failedCheckout := span("checkout-error", "checkout", incidentTrace, "dddddddddddddddd", "", "POST /checkout", "ERROR", seed.Add(2*time.Minute), 250*time.Millisecond, 2, 500)
	failedCheckout.ErrorType = "CheckoutError"
	failedCheckout.ErrorMessage = "checkout dependency failed"
	machineLog := log("machine-auth-log", "file-agent", "", "WARN", "failed password for invalid user admin", seed.Add(2*time.Minute+30*time.Second))
	machineLog.ResourceAttributes = map[string]any{
		"service.name":    "file-agent",
		"host.name":       "demo-host-01",
		"log.file.path":   "/var/log/auth.log",
		"log.source.type": "file",
	}
	raw := []telemetry.Event{
		span("checkout-root", "checkout", checkoutTrace, "bbbbbbbbbbbbbbbb", "", "GET /checkout", "OK", seed, 120*time.Millisecond, 2, 200),
		failedPayment,
		span("catalog-root", "catalog", catalogTrace, "cccccccccccccccc", "", "GET /products", "OK", seed.Add(time.Minute), 45*time.Millisecond, 2, 200),
		log("catalog-log", "catalog", catalogTrace, "INFO", "catalog cache refreshed", seed.Add(time.Minute+50*time.Millisecond)),
		failedCheckout,
		machineLog,
		log("checkout-log", "checkout", incidentTrace, "ERROR", "checkout returned 500", seed.Add(2*time.Minute+250*time.Millisecond)),
		{
			ID:                 telemetry.DeterministicID(seed, "mock", "firewall-cef"),
			ProjectID:          ingestion.ProjectID,
			Environment:        ingestion.EnvironmentName,
			ServiceName:        "syslog",
			Type:               telemetry.EventLog,
			Timestamp:          seed.Add(3 * time.Minute),
			Severity:           "HIGH",
			Signal:             telemetry.SignalLog,
			Attributes:         map[string]any{"source": "tracepilot-mockdata", "log.source.type": "syslog"},
			ResourceAttributes: resourceAttributes("syslog"),
			ScopeAttributes:    map[string]any{},
			RecordAttributes:   map[string]any{"log.source.type": "syslog"},
			LogBody:            "<134>1 2026-08-31T12:03:00Z demo-firewall - - - CEF:0|TracePilot Demo|Firewall|1|1001|Blocked outbound connection|8|src=10.0.0.8 dst=203.0.113.5 spt=52123 dpt=443 act=blocked outcome=failure proto=TCP msg=policy denied",
		},
	}
	// Normalize security events (CEF/LEEF/syslog parsing) so the Security field
	// is populated before the events are stored. The live server does the same
	// via the pipeline's StoreFunc; mockdata calls StoreEvents directly so it
	// must apply the same processing step here.
	return securityingest.Process(raw)
}
