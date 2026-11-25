package traces

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

const (
	testProjectID = "01900000-0000-7000-8000-0000000000c0"
	pagedTraceID  = "0123456789abcdef0123456789abcdef"
)

var principal = auth.Principal{UserID: "user", ProjectID: testProjectID, EnvironmentID: "environment"}

type backend struct {
	spans      []query.SpanRecord
	hasMore    bool
	err        error
	page       query.SpanPage
	project    string
	traceID    string
	listFilter query.TraceFilter
}

func (backend *backend) ListTraces(_ context.Context, project string, filter query.TraceFilter) ([]query.TraceSummary, error) {
	backend.project, backend.listFilter = project, filter
	return nil, nil
}

func (backend *backend) GetTrace(_ context.Context, project, _, traceID string, page query.SpanPage) ([]query.SpanRecord, bool, error) {
	backend.project, backend.page, backend.traceID = project, page, traceID
	return backend.spans, backend.hasMore, backend.err
}

func spanAt(second int, id string) query.SpanRecord {
	return query.SpanRecord{EventID: id, StartedAt: time.Date(2026, 10, 1, 0, 0, second, 0, time.UTC)}
}

func getTrace(backend *backend, traceID, queryString string) (*httptest.ResponseRecorder, map[string]any) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/traces/"+traceID+queryString, nil)
	request.SetPathValue("traceID", traceID)
	recorder := httptest.NewRecorder()
	New(backend, nil).get(recorder, request, principal)
	var body map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	return recorder, body
}

func TestTraceResponseSaysWhenMoreSpansFollow(t *testing.T) {
	backend := &backend{spans: []query.SpanRecord{spanAt(1, "01900000-0000-7000-8000-000000000001"), spanAt(2, "01900000-0000-7000-8000-000000000002")}, hasMore: true}
	recorder, body := getTrace(backend, pagedTraceID, "")
	if recorder.Code != http.StatusOK || backend.page.Limit != MaxSpansPerPage || backend.page.MaxBytes != maxPageBytes || backend.project != testProjectID {
		t.Fatalf("status=%d page=%+v project=%q", recorder.Code, backend.page, backend.project)
	}
	cursor, ok := body["next_cursor"].(string)
	if !ok || cursor == "" {
		t.Fatalf("a partial trace was returned without next_cursor: %v", body)
	}

	backend.hasMore = false
	_, body = getTrace(backend, pagedTraceID, "?limit=2&cursor="+cursor)
	if _, present := body["next_cursor"]; present {
		t.Fatal("next_cursor was sent for the final page")
	}
	if backend.page.Limit != 2 || backend.page.AfterEventID != "01900000-0000-7000-8000-000000000002" || !backend.page.AfterStartedAt.Equal(spanAt(2, "").StartedAt) {
		t.Fatalf("the cursor did not resume after the last span: %+v", backend.page)
	}
}

func TestTraceRejectsLimitsAndCursorsItCannotHonor(t *testing.T) {
	for _, queryString := range []string{"?limit=0", "?limit=501", "?limit=abc", "?cursor=garbage", "?cursor=" + render.EncodeCursor(render.PageCursor{At: time.Now(), ID: "not-a-uuid"})} {
		backend := &backend{}
		if recorder, _ := getTrace(backend, pagedTraceID, queryString); recorder.Code != http.StatusBadRequest || backend.page.Limit != 0 {
			t.Fatalf("%s: status %d (backend queried: %t)", queryString, recorder.Code, backend.page.Limit != 0)
		}
	}
	if recorder, _ := getTrace(&backend{}, "not-hex", ""); recorder.Code != http.StatusBadRequest {
		t.Fatalf("malformed trace ID: status %d", recorder.Code)
	}
}

func TestMissingTraceIs404OnlyOnTheFirstPage(t *testing.T) {
	if recorder, _ := getTrace(&backend{}, pagedTraceID, ""); recorder.Code != http.StatusNotFound {
		t.Fatalf("first page of an unknown trace: status %d", recorder.Code)
	}
	cursor := render.EncodeCursor(render.PageCursor{At: time.Now(), ID: "01900000-0000-7000-8000-000000000002"})
	recorder, body := getTrace(&backend{}, pagedTraceID, "?cursor="+cursor)
	if recorder.Code != http.StatusOK || body["spans"] == nil {
		t.Fatalf("an emptied later page must be an empty 200, got %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestSlowTraceQueryTimesOutWithoutLeakingDetails(t *testing.T) {
	recorder, _ := getTrace(&backend{err: context.DeadlineExceeded}, pagedTraceID, "")
	if recorder.Code != http.StatusGatewayTimeout || !strings.Contains(recorder.Body.String(), "QUERY_TIMEOUT") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	recorder, _ = getTrace(&backend{err: errors.New("pq: relation spans_p20261001 is corrupt")}, pagedTraceID, "")
	if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), "corrupt") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestTraceReadsNormalizeHexIDs(t *testing.T) {
	backend := &backend{spans: []query.SpanRecord{spanAt(1, "01900000-0000-7000-8000-000000000001")}}
	recorder, body := getTrace(backend, strings.ToUpper(pagedTraceID), "")
	if recorder.Code != http.StatusOK || body["trace_id"] != pagedTraceID || backend.traceID != pagedTraceID {
		t.Fatalf("trace ID sent to storage %q and returned %v", backend.traceID, body["trace_id"])
	}
	upper := strings.ToUpper(pagedTraceID)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/traces?traceId="+upper+"&cursor="+render.EncodeCursor(render.PageCursor{At: time.Now(), ID: upper}), nil)
	filter, cursor, err := parseFilter(request)
	if err != nil || filter.TraceID != pagedTraceID || cursor.ID != pagedTraceID {
		t.Fatalf("trace list filter or cursor not normalized: %+v %+v %v", filter, cursor, err)
	}
}

func TestListTracesWithoutFromReadsOnlyTheLastHour(t *testing.T) {
	backend := &backend{}
	recorder := httptest.NewRecorder()
	before := time.Now()
	New(backend, nil).list(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/traces?limit=1", nil), principal)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	earliest, latest := before.Add(-render.DefaultWindow), time.Now().Add(-render.DefaultWindow)
	filter := backend.listFilter
	if filter.From.Before(earliest) || filter.From.After(latest) || !filter.To.IsZero() || filter.EnvironmentID != "environment" || backend.project != testProjectID {
		t.Fatalf("open-ended list reached storage unbounded or unscoped: %+v project %q", filter, backend.project)
	}
}

func TestParseDurationFilters(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/traces?minDuration=250ms&maxDuration=5s", nil)
	filter, _, err := parseFilter(request)
	if err != nil {
		t.Fatal(err)
	}
	if filter.MinDurationNs == nil || *filter.MinDurationNs != int64(250*time.Millisecond) {
		t.Fatalf("min duration = %#v", filter.MinDurationNs)
	}
	if filter.MaxDurationNs == nil || *filter.MaxDurationNs != int64(5*time.Second) {
		t.Fatalf("max duration = %#v", filter.MaxDurationNs)
	}
	for _, path := range []string{"/api/v1/traces?minDuration=5s&maxDuration=1s", "/api/v1/traces?minDuration=-1s"} {
		if _, _, err := parseFilter(httptest.NewRequest(http.MethodGet, path, nil)); err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
}

func TestParseFilterRejectsMalformedIDs(t *testing.T) {
	cursor := render.EncodeCursor(render.PageCursor{At: time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC), ID: "not-an-id"})
	for _, path := range []string{"/api/v1/traces?cursor=" + cursor, "/api/v1/traces?traceId=not-hex"} {
		if _, _, err := parseFilter(httptest.NewRequest(http.MethodGet, path, nil)); err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
}
