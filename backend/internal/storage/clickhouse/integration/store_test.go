// Package integration runs the ClickHouse stores against a disposable ClickHouse server. Set
// TRACEPILOT_CLICKHOUSE_INTEGRATION_TEST=1 and TRACEPILOT_CLICKHOUSE_TEST_URL to run it.
package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry/telemetrytest"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"
)

const (
	testProject     = telemetrytest.Project
	testEnvironment = telemetrytest.Environment
)

// testStore is the joined store plus a raw client for statements the stores do not expose.
type testStore struct {
	*clickhouse.Store
	raw *client.Client
}

// integrationStore migrates a fresh database and drops it when the test ends.
func integrationStore(t *testing.T) *testStore {
	t.Helper()
	if os.Getenv("TRACEPILOT_CLICKHOUSE_INTEGRATION_TEST") != "1" {
		t.Skip("set TRACEPILOT_CLICKHOUSE_INTEGRATION_TEST=1 and TRACEPILOT_CLICKHOUSE_TEST_URL to a disposable ClickHouse")
	}
	configuration := config.ClickHouseConfig{URL: os.Getenv("TRACEPILOT_CLICKHOUSE_TEST_URL"),
		Database: fmt.Sprintf("tracepilot_test_%d", time.Now().UnixNano()), Username: "default",
		RequestTimeout: 10 * time.Second, MaxConnections: 2}
	store, err := clickhouse.New(configuration)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := client.New(configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := raw.Execute(context.Background(), "DROP DATABASE `"+configuration.Database+"`", nil, nil, false); err != nil {
			t.Error(err)
		}
		raw.Close()
		store.Close()
	})
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return &testStore{Store: store, raw: raw}
}
