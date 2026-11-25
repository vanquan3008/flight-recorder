// Package clienttest starts a fake ClickHouse HTTP server for tests of the stores.
package clienttest

import (
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry/telemetrytest"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"
)

// Config serves handler as ClickHouse and returns a configuration that reaches it, changed by adjust.
func Config(t *testing.T, handler http.HandlerFunc, adjust ...func(*config.ClickHouseConfig)) config.ClickHouseConfig {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	configuration := config.ClickHouseConfig{URL: server.URL, Database: "test", Username: "default", Password: "private",
		RequestTimeout: time.Second, MaxConnections: 2}
	for _, change := range adjust {
		change(&configuration)
	}
	return configuration
}

// Client is a client connected to the fake server.
func Client(t *testing.T, handler http.HandlerFunc, adjust ...func(*config.ClickHouseConfig)) *client.Client {
	t.Helper()
	connection, err := client.New(Config(t, handler, adjust...))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(connection.Close)
	return connection
}

// DecodeRows reads the JSONEachRow rows of a compressed insert.
func DecodeRows[T any](t *testing.T, r *http.Request) []T {
	t.Helper()
	reader, err := gzip.NewReader(r.Body)
	if err != nil {
		t.Error(err)
		return nil
	}
	defer reader.Close()
	var rows []T
	decoder := json.NewDecoder(reader)
	for decoder.More() {
		var row T
		if err := decoder.Decode(&row); err != nil {
			t.Error(err)
			return rows
		}
		rows = append(rows, row)
	}
	return rows
}

// Span is a failed database span of 1.5 ms in the test project.
func Span(at time.Time, identity string) telemetry.Event {
	return telemetry.Event{ID: telemetry.DeterministicID(at, identity), ProjectID: telemetrytest.Project, EnvironmentID: telemetrytest.Environment,
		ServiceName: "api", Signal: telemetry.SignalTrace, Type: telemetry.EventHTTPClient, Timestamp: at, Duration: 1500 * time.Microsecond,
		TraceID: "0123456789abcdef0123456789abcdef", SpanID: "0123456789abcdef", Operation: "SELECT orders", SpanKind: 3, Status: "ERROR",
		RecordAttributes: map[string]any{"db.system": "postgresql", "db.namespace": "orders"},
		SpanEvents:       []telemetry.SpanEvent{{Timestamp: at, Name: "exception", Attributes: map[string]any{"exception.type": "Timeout"}}},
	}
}

// Log is an INFO log record in the test project.
func Log(at time.Time, identity string) telemetry.Event {
	return telemetry.Event{ID: telemetry.DeterministicID(at, identity), ProjectID: telemetrytest.Project, EnvironmentID: telemetrytest.Environment,
		ServiceName: "api", Signal: telemetry.SignalLog, Type: telemetry.EventLog, Timestamp: at, SeverityNumber: 9,
		LogBody: map[string]any{"message": "đặt hàng\t\"ok\""}, RecordAttributes: map[string]any{"request.id": "r1"}}
}

// DropDatabase removes a disposable test database from a real ClickHouse server.
func DropDatabase(t *testing.T, cfg config.ClickHouseConfig) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, cfg.URL+"/?query="+url.QueryEscape("DROP DATABASE IF EXISTS `"+cfg.Database+"`"), nil)
	if err != nil {
		t.Error(err)
		return
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		t.Error(err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Error("ClickHouse test cleanup failed", response.StatusCode)
	}
}

// Store builds a store with construct over a client connected to the fake server.
func Store[T any](t *testing.T, construct func(*client.Client) *T, handler http.HandlerFunc, adjust ...func(*config.ClickHouseConfig)) *T {
	t.Helper()
	return construct(Client(t, handler, adjust...))
}
