package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi"
)

// emptyTelemetry lets the metadata API tests run with just PostgreSQL.
type emptyTelemetry struct{}

func postgresBackends(database *postgres.Database) httpapi.Backends {
	return httpapi.Backends{
		Access: database, Session: database, Users: database, Projects: database, ProjectRetention: database,
		Members: database, IngestTokens: database, Incidents: database, Webhooks: database,
		Traces: emptyTelemetry{}, Logs: emptyTelemetry{}, Services: emptyTelemetry{},
	}
}

func (emptyTelemetry) ListLogs(context.Context, string, query.LogFilter) ([]query.LogRecord, error) {
	return nil, nil
}

func (emptyTelemetry) GetLog(context.Context, string, string, string) (query.LogRecord, error) {
	return query.LogRecord{}, nil
}

func (emptyTelemetry) ListTraces(context.Context, string, query.TraceFilter) ([]query.TraceSummary, error) {
	return []query.TraceSummary{}, nil
}

func (emptyTelemetry) GetTrace(context.Context, string, string, string, query.SpanPage) ([]query.SpanRecord, bool, error) {
	return nil, false, nil
}

func (emptyTelemetry) ListServices(context.Context, string, string, time.Time, time.Time) ([]query.ServiceSummary, error) {
	return nil, nil
}

func (emptyTelemetry) ListServiceDependencies(context.Context, string, string, string) ([]query.ServiceDependency, error) {
	return nil, nil
}

func newQueryIntegrationDatabase(t *testing.T) (*postgres.Database, config.IngestionConfig, string) {
	t.Helper()
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("set TRACEPILOT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	databaseURL := os.Getenv("TRACEPILOT_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TRACEPILOT_DATABASE_URL is required")
	}
	database, err := postgres.Open(context.Background(), config.DatabaseConfig{
		URL:                   databaseURL,
		MaxConnections:        4,
		MaxConnectionLifetime: time.Minute,
		MaxConnectionIdleTime: time.Minute,
		HealthCheckPeriod:     time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)

	runID := time.Now().UTC().Format(time.RFC3339Nano)
	projectID := telemetry.DeterministicID(time.Now().UTC(), "query-integration-project", runID)
	ingestion := config.IngestionConfig{
		ProjectID:       projectID,
		ProjectSlug:     "query-" + projectID[24:32],
		ProjectName:     "Query Integration",
		EnvironmentID:   telemetry.DeterministicID(time.Now().UTC(), "query-integration-environment", runID),
		EnvironmentName: "test",
	}
	if err := database.EnsureIngestionScope(context.Background(), ingestion); err != nil {
		t.Fatal(err)
	}
	return database, ingestion, newIntegrationAdmin(t, database)
}
