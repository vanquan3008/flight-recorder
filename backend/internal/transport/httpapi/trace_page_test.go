package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/query"
)

const pagedTraceID = "0123456789abcdef0123456789abcdef"

type tracePageBackend struct {
	authOnlyBackend
	spans     []query.SpanRecord
	hasMore   bool
	err       error
	page      query.SpanPage
	project   string
	traceID   string
	logFilter query.LogFilter
}

func (backend *tracePageBackend) GetTrace(_ context.Context, project, _, traceID string, page query.SpanPage) ([]query.SpanRecord, bool, error) {
	backend.project, backend.page, backend.traceID = project, page, traceID
	return backend.spans, backend.hasMore, backend.err
}

func (backend *tracePageBackend) ListLogs(_ context.Context, _ string, filter query.LogFilter) ([]query.LogRecord, error) {
	backend.logFilter = filter
	return nil, nil
}

func TestTraceAndLogReadsNormalizeHexIDs(t *testing.T) {
	const spanID = "0123456789abcdef"
	backend := &tracePageBackend{spans: []query.SpanRecord{spanAt(1, "01900000-0000-7000-8000-000000000001")}}
	server := newTestServer(nil)
	server.backend = backend
	router := http.NewServeMux()
	server.registerAPIRoutes(router)
	get := func(path string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		signIn(request, "user")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, response.Code, response.Body.String())
		}
		return response
	}
	upperTrace, upperSpan := strings.ToUpper(pagedTraceID), strings.ToUpper(spanID)
	response := get("/api/v1/traces/" + upperTrace)
	var trace traceResponse
	if err := json.Unmarshal(response.Body.Bytes(), &trace); err != nil || trace.TraceID != pagedTraceID || backend.traceID != pagedTraceID {
		t.Fatalf("trace ID sent to storage %q and returned %q, decode error: %v", backend.traceID, trace.TraceID, err)
	}
	for _, path := range []string{
		"/api/v1/traces/" + upperTrace + "/logs?spanId=" + upperSpan,
		"/api/v1/logs?traceId=" + upperTrace + "&spanId=" + upperSpan,
	} {
		get(path)
		if backend.logFilter.TraceID != pagedTraceID || backend.logFilter.SpanID != spanID {
			t.Fatalf("log filters reached storage without normalization: %+v", backend.logFilter)
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/traces?traceId="+upperTrace+"&cursor="+encodeCursor(pageCursor{At: time.Now(), ID: upperTrace}), nil)
	filter, cursor, err := parseTraceFilter(request)
	if err != nil || filter.TraceID != pagedTraceID || cursor.ID != pagedTraceID {
		t.Fatalf("trace list filter or cursor not normalized: %+v %+v %v", filter, cursor, err)
	}
}

func getTracePage(t *testing.T, backend *tracePageBackend, queryString string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	server := newTestServer(nil)
	server.backend = backend
	router := http.NewServeMux()
	server.registerAPIRoutes(router)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/traces/"+pagedTraceID+queryString, nil)
	signIn(request, "user")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var body map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	return recorder, body
}

func spanAt(second int, id string) query.SpanRecord {
	return query.SpanRecord{EventID: id, StartedAt: time.Date(2026, 10, 1, 0, 0, second, 0, time.UTC)}
}

func TestTraceResponseSaysWhenMoreSpansFollow(t *testing.T) {
	backend := &tracePageBackend{spans: []query.SpanRecord{spanAt(1, "01900000-0000-7000-8000-000000000001"), spanAt(2, "01900000-0000-7000-8000-000000000002")}, hasMore: true}
	recorder, body := getTracePage(t, backend, "")
	if recorder.Code != http.StatusOK || backend.page.Limit != maxTraceSpansPerPage || backend.page.MaxBytes != maxTracePageBytes || backend.project != testProjectIDForRoutes {
		t.Fatalf("status=%d page=%+v project=%q", recorder.Code, backend.page, backend.project)
	}
	cursor, ok := body["next_cursor"].(string)
	if !ok || cursor == "" {
		t.Fatalf("a partial trace was returned without next_cursor: %v", body)
	}

	backend.hasMore = false
	_, body = getTracePage(t, backend, "?limit=2&cursor="+cursor)
	if _, present := body["next_cursor"]; present {
		t.Fatal("next_cursor was sent for the final page")
	}
	if backend.page.Limit != 2 || backend.page.AfterEventID != "01900000-0000-7000-8000-000000000002" || !backend.page.AfterStartedAt.Equal(spanAt(2, "").StartedAt) {
		t.Fatalf("the cursor did not resume after the last span: %+v", backend.page)
	}
}

func TestTraceRejectsLimitsAndCursorsItCannotHonor(t *testing.T) {
	for _, queryString := range []string{"?limit=0", "?limit=501", "?limit=abc", "?cursor=garbage", "?cursor=" + encodeCursor(pageCursor{At: time.Now(), ID: "not-a-uuid"})} {
		backend := &tracePageBackend{}
		if recorder, _ := getTracePage(t, backend, queryString); recorder.Code != http.StatusBadRequest || backend.page.Limit != 0 {
			t.Fatalf("%s: status %d (backend queried: %t)", queryString, recorder.Code, backend.page.Limit != 0)
		}
	}
}

func TestMissingTraceIs404OnlyOnTheFirstPage(t *testing.T) {
	if recorder, _ := getTracePage(t, &tracePageBackend{}, ""); recorder.Code != http.StatusNotFound {
		t.Fatalf("first page of an unknown trace: status %d", recorder.Code)
	}
	cursor := encodeCursor(pageCursor{At: time.Now(), ID: "01900000-0000-7000-8000-000000000002"})
	recorder, body := getTracePage(t, &tracePageBackend{}, "?cursor="+cursor)
	if recorder.Code != http.StatusOK || body["spans"] == nil {
		t.Fatalf("an emptied later page must be an empty 200, got %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestSlowTraceQueryTimesOutWithoutLeakingDetails(t *testing.T) {
	recorder, _ := getTracePage(t, &tracePageBackend{err: context.DeadlineExceeded}, "")
	if recorder.Code != http.StatusGatewayTimeout || !strings.Contains(recorder.Body.String(), "QUERY_TIMEOUT") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	recorder, _ = getTracePage(t, &tracePageBackend{err: errors.New("pq: relation spans_p20261001 is corrupt")}, "")
	if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), "corrupt") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
