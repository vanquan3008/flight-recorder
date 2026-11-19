package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/query"
)

func TestDefaultTraceWindowBoundsOpenEndedLists(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	from, to, before := now.Add(-6*time.Hour), now.Add(-2*time.Hour), now.Add(-30*time.Minute)
	for _, testCase := range []struct {
		name   string
		filter query.TraceFilter
		want   time.Time
	}{
		{"no range ends now", query.TraceFilter{}, now.Add(-time.Hour)},
		{"only to ends at to", query.TraceFilter{To: to}, to.Add(-time.Hour)},
		{"a later page ends at its cursor", query.TraceFilter{Before: before}, before.Add(-time.Hour)},
		{"to wins over the cursor", query.TraceFilter{To: to, Before: before}, to.Add(-time.Hour)},
		{"an explicit from is kept", query.TraceFilter{From: from}, from},
		{"a trace ID lookup stays unbounded", query.TraceFilter{TraceID: pagedTraceID}, time.Time{}},
	} {
		filter := testCase.filter
		applyDefaultTraceWindow(&filter, now)
		if !filter.From.Equal(testCase.want) {
			t.Errorf("%s: from = %s, want %s", testCase.name, filter.From, testCase.want)
		}
	}
}

func TestDefaultLogWindowBoundsOpenEndedLists(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	from, to, before := now.Add(-6*time.Hour), now.Add(-2*time.Hour), now.Add(-30*time.Minute)
	for _, testCase := range []struct {
		name   string
		filter query.LogFilter
		want   time.Time
	}{
		{"no range ends now", query.LogFilter{}, now.Add(-time.Hour)},
		{"only to ends at to", query.LogFilter{To: to}, to.Add(-time.Hour)},
		{"a later page ends at its cursor", query.LogFilter{Before: before}, before.Add(-time.Hour)},
		{"an explicit from is kept", query.LogFilter{From: from}, from},
		{"a trace's logs stay unbounded", query.LogFilter{TraceID: pagedTraceID}, time.Time{}},
	} {
		filter := testCase.filter
		applyDefaultLogWindow(&filter, now)
		if !filter.From.Equal(testCase.want) {
			t.Errorf("%s: from = %s, want %s", testCase.name, filter.From, testCase.want)
		}
	}
}

type traceListBackend struct {
	authOnlyBackend
	filter query.TraceFilter
}

func (backend *traceListBackend) ListTraces(_ context.Context, _ string, filter query.TraceFilter) ([]query.TraceSummary, error) {
	backend.filter = filter
	return nil, nil
}

func TestListTracesWithoutFromReadsOnlyTheLastHour(t *testing.T) {
	backend := &traceListBackend{}
	server := newTestServer(nil)
	server.backend = backend
	router := http.NewServeMux()
	server.registerAPIRoutes(router)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/traces?limit=1", nil)
	signIn(request, "user")
	recorder := httptest.NewRecorder()
	before := time.Now()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	earliest, latest := before.Add(-defaultTraceWindow), time.Now().Add(-defaultTraceWindow)
	if backend.filter.From.Before(earliest) || backend.filter.From.After(latest) || !backend.filter.To.IsZero() {
		t.Fatalf("open-ended list reached storage unbounded: from=%s to=%s", backend.filter.From, backend.filter.To)
	}
}
