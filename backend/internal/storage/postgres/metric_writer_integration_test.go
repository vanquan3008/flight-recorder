package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func TestStoreMetricsIsScopedAndIdempotent(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("set TRACEPILOT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	databaseURL := os.Getenv("TRACEPILOT_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TRACEPILOT_DATABASE_URL is required")
	}
	database, openError := Open(context.Background(), config.DatabaseConfig{
		URL: databaseURL, MaxConnections: 4, MaxConnectionLifetime: time.Minute,
		MaxConnectionIdleTime: time.Minute, HealthCheckPeriod: time.Minute,
	})
	if openError != nil {
		t.Fatal(openError)
	}
	t.Cleanup(database.Close)

	runID := time.Now().UTC().Format(time.RFC3339Nano)
	projectID := telemetry.DeterministicID(time.Now().UTC(), "metrics-project", runID)
	ingestion := config.IngestionConfig{
		ProjectID: projectID, ProjectSlug: "metrics-" + projectID[24:32], ProjectName: "Metrics Integration",
		EnvironmentID: telemetry.DeterministicID(time.Now().UTC(), "metrics-environment", runID), EnvironmentName: "test",
	}
	if scopeError := database.EnsureIngestionScope(context.Background(), ingestion); scopeError != nil {
		t.Fatal(scopeError)
	}
	point := telemetry.MetricDataPoint{
		ID: telemetry.DeterministicID(time.Now().UTC(), "metric-point", runID), ProjectID: ingestion.ProjectID,
		EnvironmentID: ingestion.EnvironmentID, MetricName: "checkout.requests", MetricType: telemetry.MetricTypeGauge,
		ServiceName: "checkout", OccurredAt: time.Now().UTC(), Temporality: "cumulative",
		Attributes: map[string]any{"region": "sg"}, ResourceAttributes: map[string]any{"service.name": "checkout"},
	}
	value := 42.0
	point.Value = &value
	if storeError := database.StoreMetrics(context.Background(), ingestion, []telemetry.MetricDataPoint{point}); storeError != nil {
		t.Fatal(storeError)
	}
	if storeError := database.StoreMetrics(context.Background(), ingestion, []telemetry.MetricDataPoint{point}); storeError != nil {
		t.Fatal(storeError)
	}
	var count int
	if queryError := database.pool.QueryRow(context.Background(), `SELECT count(*) FROM metric_data_points WHERE project_id=$1 AND id=$2`, ingestion.ProjectID, point.ID).Scan(&count); queryError != nil {
		t.Fatal(queryError)
	}
	if count != 1 {
		t.Fatalf("metric rows=%d, want 1 after retry", count)
	}
	other := point
	other.EnvironmentID = telemetry.DeterministicID(time.Now().UTC(), "metrics-other-environment", runID)
	if storeError := database.StoreMetrics(context.Background(), ingestion, []telemetry.MetricDataPoint{other}); storeError == nil {
		t.Fatal("cross-environment metric was accepted")
	}
}
