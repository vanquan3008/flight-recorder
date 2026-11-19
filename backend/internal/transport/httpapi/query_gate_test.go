package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"github.com/vanquan3008/tracepilot/internal/query"
)

// slowServicesBackend answers ListServices only when told to, or when the
// request's context ends first.
type slowServicesBackend struct {
	authOnlyBackend
	release   chan struct{}
	started   chan struct{}
	cancelled chan struct{}
}

func (backend *slowServicesBackend) ListServices(ctx context.Context, _, _ string, _, _ time.Time) ([]query.ServiceSummary, error) {
	backend.started <- struct{}{}
	select {
	case <-backend.release:
		return []query.ServiceSummary{}, nil
	case <-ctx.Done():
		backend.cancelled <- struct{}{}
		return nil, ctx.Err()
	}
}

func gatedServer(backend *slowServicesBackend, concurrent int, timeout time.Duration) http.Handler {
	configuration := config.Config{
		HTTP:            config.HTTPConfig{ManagementMaxBodyBytes: 1024, MaxConcurrentQueries: concurrent, RequestTimeout: timeout},
		Auth:            config.AuthConfig{JWTSecret: testJWTSecret},
		Database:        config.DatabaseConfig{ReadinessTimeout: time.Second},
		ShutdownTimeout: time.Second,
	}
	server := New(configuration, slog.New(slog.NewTextHandler(io.Discard, nil)), func(context.Context) error { return nil }, metrics.New("test", "test", "test"), backend, nil)
	return server.httpServer.Handler
}

func getServices(handler http.Handler) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/services", nil)
	signIn(request, "user")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestSlowReadsCannotTakeMoreThanTheirShareOfSlots(t *testing.T) {
	backend := &slowServicesBackend{release: make(chan struct{}), started: make(chan struct{}, 8), cancelled: make(chan struct{}, 8)}
	handler := gatedServer(backend, 2, time.Minute)
	var running sync.WaitGroup
	for range 2 {
		running.Add(1)
		go func() { defer running.Done(); getServices(handler) }()
	}
	<-backend.started
	<-backend.started // both slots are now held by queries that will not finish

	refusedAt := time.Now()
	refused := getServices(handler)
	if refused.Code != http.StatusServiceUnavailable || refused.Header().Get("Retry-After") == "" || !strings.Contains(refused.Body.String(), "OVERLOADED") {
		t.Fatalf("third request: status=%d retry-after=%q body=%s", refused.Code, refused.Header().Get("Retry-After"), refused.Body.String())
	}
	if waited := time.Since(refusedAt); waited > 2*time.Second {
		t.Fatalf("an overloaded API took %s to refuse, it must fail fast", waited)
	}

	close(backend.release) // finishes the two held queries, and any later one answers at once
	running.Wait()
	if again := getServices(handler); again.Code != http.StatusOK {
		t.Fatalf("slots were not returned after the queries finished: status %d", again.Code)
	}
}

type slowServicesFastLogsBackend struct {
	*slowServicesBackend
}

func (slowServicesFastLogsBackend) ListLogs(context.Context, string, query.LogFilter) ([]query.LogRecord, error) {
	return []query.LogRecord{}, nil
}

func TestLogReadsDoNotQueueBehindSlowQueries(t *testing.T) {
	slow := &slowServicesBackend{release: make(chan struct{}), started: make(chan struct{}, 8), cancelled: make(chan struct{}, 8)}
	configuration := config.Config{
		HTTP:            config.HTTPConfig{ManagementMaxBodyBytes: 1024, MaxConcurrentQueries: 1, MaxConcurrentLogQueries: 1, RequestTimeout: time.Minute},
		Auth:            config.AuthConfig{JWTSecret: testJWTSecret},
		Database:        config.DatabaseConfig{ReadinessTimeout: time.Second},
		ShutdownTimeout: time.Second,
	}
	server := New(configuration, slog.New(slog.NewTextHandler(io.Discard, nil)), func(context.Context) error { return nil }, metrics.New("test", "test", "test"), slowServicesFastLogsBackend{slow}, nil)
	handler := server.httpServer.Handler
	var running sync.WaitGroup
	running.Add(1)
	go func() { defer running.Done(); getServices(handler) }()
	<-slow.started // the only query slot is now held

	request := httptest.NewRequest(http.MethodGet, "/api/v1/logs?limit=1", nil)
	signIn(request, "user")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	close(slow.release)
	running.Wait()
	if recorder.Code != http.StatusOK {
		t.Fatalf("a log read waited for the PostgreSQL query gate: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestRequestDeadlineCancelsTheQueryAndFreesItsSlot(t *testing.T) {
	backend := &slowServicesBackend{release: make(chan struct{}), started: make(chan struct{}, 8), cancelled: make(chan struct{}, 8)}
	handler := gatedServer(backend, 1, 50*time.Millisecond)
	slow := getServices(handler)
	if slow.Code != http.StatusGatewayTimeout || !strings.Contains(slow.Body.String(), "QUERY_TIMEOUT") {
		t.Fatalf("slow query: status=%d body=%s", slow.Code, slow.Body.String())
	}
	select {
	case <-backend.cancelled:
	default:
		t.Fatal("the database call was not cancelled; a timed-out request would still hold its connection")
	}
	close(backend.release)
	if next := getServices(handler); next.Code != http.StatusOK {
		t.Fatalf("the slot of a timed-out request was not freed: status %d", next.Code)
	}
}
