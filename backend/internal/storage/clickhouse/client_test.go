package clickhouse

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
)

const testProject = "01900000-0000-7000-8000-000000000001"
const testEnvironment = "01900000-0000-7000-8000-000000000002"

func testStore(t *testing.T, handler http.HandlerFunc) *Store {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	store, err := New(config.ClickHouseConfig{URL: server.URL, Database: "test", Username: "default", Password: "private",
		RequestTimeout: time.Second, MaxConnections: 2}, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	return store
}

func testLog(at time.Time, identity string) telemetry.Event {
	return telemetry.Event{ID: telemetry.DeterministicID(at, identity), ProjectID: testProject, EnvironmentID: testEnvironment,
		ServiceName: "api", Signal: telemetry.SignalLog, Type: telemetry.EventLog, Timestamp: at, SeverityNumber: 9,
		LogBody: map[string]any{"message": "đặt hàng\t\"ok\""}, RecordAttributes: map[string]any{"request.id": "r1"}}
}

func TestInsertUsesCompressedBatchAndWaitsForFlush(t *testing.T) {
	called := false
	store := testStore(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		q := r.URL.Query()
		if r.Method != "POST" || q.Get("async_insert") != "1" || q.Get("wait_for_async_insert") != "1" ||
			r.Header.Get("Content-Encoding") != "gzip" || r.Header.Get("X-ClickHouse-Key") != "private" {
			t.Error("insert must be authenticated, compressed and wait for persistence")
		}
		reader, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		defer reader.Close()
		var row storedLog
		if err := json.NewDecoder(reader).Decode(&row); err != nil || row.ProjectID != testProject || !json.Valid([]byte(row.Body)) {
			t.Errorf("bad log payload: %+v, %v", row, err)
		}
	})
	if err := store.StoreLogs(context.Background(), testProject, testEnvironment, []telemetry.Event{testLog(time.Now().UTC(), "insert")}); err != nil || !called {
		t.Fatalf("insert error=%v called=%t", err, called)
	}
}

func TestQueryUsesTypedEscapedParametersAndFinal(t *testing.T) {
	service := "api\t' OR 1=1 --\\line\n"
	store := testStore(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("param_service") != parameter(service) || strings.Contains(q.Get("query"), service) ||
			!strings.Contains(q.Get("query"), "FINAL") || q.Get("param_project") != testProject || q.Get("param_limit") != "3" {
			t.Error("query must scope, bind filters, deduplicate and read one extra row")
		}
	})
	if _, err := store.ListLogs(context.Background(), testProject, query.LogFilter{EnvironmentID: testEnvironment, Service: service, Limit: 2}); err != nil {
		t.Fatal(err)
	}
}

func TestServerFailuresCannotAcknowledgeOrLeakContent(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusInternalServerError} {
		store := testStore(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, "private telemetry SQL error")
		})
		err := store.StoreLogs(context.Background(), testProject, testEnvironment, []telemetry.Event{testLog(time.Now().UTC(), "failure")})
		if err == nil || strings.Contains(err.Error(), "private") {
			t.Fatalf("incorrect ACK or leaked server response: %v", err)
		}
		if _, err := store.ListLogs(context.Background(), testProject, query.LogFilter{EnvironmentID: testEnvironment, Limit: 1}); err == nil {
			t.Fatal("invalid HTTP 200 result was accepted")
		}
	}
}

func TestInvalidBatchDoesNotWriteAnyRows(t *testing.T) {
	store := testStore(t, func(http.ResponseWriter, *http.Request) { t.Error("invalid batch reached ClickHouse") })
	good := testLog(time.Now().UTC(), "valid")
	bad := good
	bad.LogBody = make(chan int)
	if err := store.StoreLogs(context.Background(), testProject, testEnvironment, []telemetry.Event{good, bad}); !errors.Is(err, telemetry.ErrInvalidEvent) {
		t.Fatalf("want permanent content failure, got %v", err)
	}
}
