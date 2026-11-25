package logs

import (
	"context"
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
	testTraceID = "0123456789abcdef0123456789abcdef"
	testSpanID  = "0123456789abcdef"
)

var principal = auth.Principal{UserID: "user", ProjectID: "project", EnvironmentID: "environment"}

type backend struct {
	filter query.LogFilter
}

func (backend *backend) ListLogs(_ context.Context, _ string, filter query.LogFilter) ([]query.LogRecord, error) {
	backend.filter = filter
	return nil, nil
}

func (*backend) GetLog(context.Context, string, string, string) (query.LogRecord, error) {
	return query.LogRecord{}, nil
}

func TestLogReadsNormalizeHexIDs(t *testing.T) {
	backend := &backend{}
	handler := New(backend, nil)
	upperTrace, upperSpan := strings.ToUpper(testTraceID), strings.ToUpper(testSpanID)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/traces/"+upperTrace+"/logs?spanId="+upperSpan, nil)
	request.SetPathValue("traceID", upperTrace)
	recorder := httptest.NewRecorder()
	handler.listForTrace(recorder, request, principal)
	if recorder.Code != http.StatusOK || backend.filter.TraceID != testTraceID || backend.filter.SpanID != testSpanID || backend.filter.EnvironmentID != "environment" {
		t.Fatalf("trace logs: %d, filter %+v", recorder.Code, backend.filter)
	}

	recorder = httptest.NewRecorder()
	handler.list(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/logs?traceId="+upperTrace+"&spanId="+upperSpan, nil), principal)
	if recorder.Code != http.StatusOK || backend.filter.TraceID != testTraceID || backend.filter.SpanID != testSpanID {
		t.Fatalf("log search: %d, filter %+v", recorder.Code, backend.filter)
	}
}

func TestMissingLogIsNotFound(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/logs/01900000-0000-7000-8000-000000000001", nil)
	request.SetPathValue("eventID", "01900000-0000-7000-8000-000000000001")
	recorder := httptest.NewRecorder()
	New(&backend{}, nil).get(recorder, request, principal)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestSearchRequiresBoundedTimeRange(t *testing.T) {
	if _, _, parseError := parseFilter(httptest.NewRequest(http.MethodGet, "/api/v1/logs?search=timeout", nil)); parseError == nil {
		t.Fatal("expected unbounded log search to fail")
	}
	from := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	to := from.Add(maxSearchRange)
	filter, _, parseError := parseFilter(httptest.NewRequest(http.MethodGet, "/api/v1/logs?search=timeout&from="+from.Format(time.RFC3339)+"&to="+to.Format(time.RFC3339), nil))
	if parseError != nil || filter.Search != "timeout" {
		t.Fatalf("filter = %#v, error = %v", filter, parseError)
	}
	tooWide := to.Add(time.Nanosecond)
	if _, _, parseError = parseFilter(httptest.NewRequest(http.MethodGet, "/api/v1/logs?search=timeout&from="+from.Format(time.RFC3339Nano)+"&to="+tooWide.Format(time.RFC3339Nano), nil)); parseError == nil {
		t.Fatal("expected oversized log search range to fail")
	}
	if _, _, parseError = parseFilter(httptest.NewRequest(http.MethodGet, "/api/v1/logs?from="+to.Format(time.RFC3339)+"&to="+from.Format(time.RFC3339), nil)); parseError == nil {
		t.Fatal("expected reversed time range to fail")
	}
}

func TestParseFilterRejectsMalformedIDs(t *testing.T) {
	cursor := render.EncodeCursor(render.PageCursor{At: time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC), ID: "not-an-id"})
	for _, path := range []string{"/api/v1/logs?cursor=" + cursor, "/api/v1/logs?traceId=not-hex", "/api/v1/logs?spanId=not-hex"} {
		if _, _, err := parseFilter(httptest.NewRequest(http.MethodGet, path, nil)); err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
}

func TestOpenEndedListsReadOnlyTheLastHourExceptForOneTrace(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		path    string
		bounded bool
	}{
		{"a log search", "/api/v1/logs", true},
		{"a trace's logs", "/api/v1/logs?traceId=" + testTraceID, false},
	} {
		backend := &backend{}
		recorder := httptest.NewRecorder()
		before := time.Now()
		New(backend, nil).list(recorder, httptest.NewRequest(http.MethodGet, testCase.path, nil), principal)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: status = %d", testCase.name, recorder.Code)
		}
		earliest, latest := before.Add(-render.DefaultWindow), time.Now().Add(-render.DefaultWindow)
		from := backend.filter.From
		if testCase.bounded && (from.Before(earliest) || from.After(latest)) || !testCase.bounded && !from.IsZero() {
			t.Fatalf("%s: from = %s", testCase.name, from)
		}
	}
}
