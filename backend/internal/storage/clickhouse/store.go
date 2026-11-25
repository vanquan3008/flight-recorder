// Package clickhouse stores and reads telemetry in ClickHouse over its HTTP interface. Each kind of
// data has its own package; Store joins them so callers see one storage.
package clickhouse

import (
	"context"
	"fmt"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/analytics"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/detection"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/logs"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/metrics"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/retention"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/rollups"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/schema"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/traces"
)

// Store reads and writes every kind of telemetry through one shared client. Its methods come from
// the embedded stores, and no two of them share a name.
type Store struct {
	client *client.Client
	*schema.Schema
	*logs.Logs
	*traces.Traces
	*metrics.Metrics
	*analytics.Analytics
	*detection.Detector
	*rollups.Rollups
	*retention.Retention
}

func New(cfg config.ClickHouseConfig) (*Store, error) {
	clickhouse, err := client.New(cfg)
	if err != nil {
		return nil, err
	}
	return &Store{
		client:    clickhouse,
		Schema:    schema.New(clickhouse),
		Logs:      logs.New(clickhouse),
		Traces:    traces.New(clickhouse),
		Metrics:   metrics.New(clickhouse),
		Analytics: analytics.New(clickhouse),
		Detector:  detection.New(clickhouse),
		Rollups:   rollups.New(clickhouse),
		Retention: retention.New(clickhouse),
	}, nil
}

func (store *Store) Close() { store.client.Close() }

// RunMigration opens a short-lived client and applies ("up") or reports ("status") the ClickHouse schema.
func RunMigration(ctx context.Context, cfg config.ClickHouseConfig, operation string) (string, error) {
	if operation != "up" && operation != "status" {
		return "", fmt.Errorf("unsupported ClickHouse migration operation %q", operation)
	}
	store, err := New(cfg)
	if err != nil {
		return "", err
	}
	defer store.Close()
	if operation == "status" {
		state, err := store.MigrationStatus(ctx)
		return state.String(), err
	}
	if err := store.Migrate(ctx); err != nil {
		return "", err
	}
	return "migrations applied", nil
}
