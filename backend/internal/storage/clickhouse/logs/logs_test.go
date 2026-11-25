package logs

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry/telemetrytest"

	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/client"
	"github.com/vanquan3008/tracepilot/internal/storage/clickhouse/clienttest"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	"github.com/vanquan3008/tracepilot/internal/query"
)

func TestInsertUsesCompressedBatchAndWaitsForFlush(t *testing.T) {
	called := false
	store := clienttest.Store(t, New, func(w http.ResponseWriter, r *http.Request) {
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
		var row struct {
			ProjectID string `json:"project_id"`
			Body      string `json:"body"`
		}
		if err := json.NewDecoder(reader).Decode(&row); err != nil || row.ProjectID != telemetrytest.Project || !json.Valid([]byte(row.Body)) {
			t.Errorf("bad log payload: %+v, %v", row, err)
		}
	})
	if err := store.StoreLogs(context.Background(), telemetrytest.Project, telemetrytest.Environment, []telemetry.Event{clienttest.Log(time.Now().UTC(), "insert")}); err != nil || !called {
		t.Fatalf("insert error=%v called=%t", err, called)
	}
}

func TestQueryUsesTypedEscapedParametersAndFinal(t *testing.T) {
	service := "api\t' OR 1=1 --\\line\n"
	store := clienttest.Store(t, New, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("param_service") != client.Parameter(service) || strings.Contains(q.Get("query"), service) ||
			!strings.Contains(q.Get("query"), "FINAL") || q.Get("param_project") != telemetrytest.Project || q.Get("param_limit") != "3" {
			t.Error("query must scope, bind filters, deduplicate and read one extra row")
		}
	})
	if _, err := store.ListLogs(context.Background(), telemetrytest.Project, query.LogFilter{EnvironmentID: telemetrytest.Environment, Service: service, Limit: 2}); err != nil {
		t.Fatal(err)
	}
}

func TestServerFailuresCannotAcknowledgeOrLeakContent(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusInternalServerError} {
		store := clienttest.Store(t, New, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, "private telemetry SQL error")
		})
		err := store.StoreLogs(context.Background(), telemetrytest.Project, telemetrytest.Environment, []telemetry.Event{clienttest.Log(time.Now().UTC(), "failure")})
		if err == nil || strings.Contains(err.Error(), "private") {
			t.Fatalf("incorrect ACK or leaked server response: %v", err)
		}
		if _, err := store.ListLogs(context.Background(), telemetrytest.Project, query.LogFilter{EnvironmentID: telemetrytest.Environment, Limit: 1}); err == nil {
			t.Fatal("invalid HTTP 200 result was accepted")
		}
	}
}

func TestServerFailureNamesOnlyTheExceptionCode(t *testing.T) {
	for header, want := range map[string]string{"241": "(HTTP 500, code 241)", "241; DROP": "(HTTP 500)"} {
		store := clienttest.Store(t, New, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-ClickHouse-Exception-Code", header)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, "Code: 241. private telemetry")
		})
		_, err := store.ListLogs(context.Background(), telemetrytest.Project, query.LogFilter{EnvironmentID: telemetrytest.Environment, Limit: 1})
		if err == nil || !strings.HasSuffix(err.Error(), want) || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "DROP") {
			t.Fatalf("header %q: error %v, want suffix %q and no response content", header, err, want)
		}
	}
}

func TestInvalidBatchDoesNotWriteAnyRows(t *testing.T) {
	store := clienttest.Store(t, New, func(http.ResponseWriter, *http.Request) { t.Error("invalid batch reached ClickHouse") })
	good := clienttest.Log(time.Now().UTC(), "valid")
	bad := good
	bad.LogBody = make(chan int)
	if err := store.StoreLogs(context.Background(), telemetrytest.Project, telemetrytest.Environment, []telemetry.Event{good, bad}); !errors.Is(err, telemetry.ErrInvalidEvent) {
		t.Fatalf("want permanent content failure, got %v", err)
	}
}
