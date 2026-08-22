package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCursorRoundTrip(t *testing.T) {
	want := pageCursor{At: time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC), ID: "01900000-0000-7000-8000-000000000001"}
	got, decodeError := decodeCursor(encodeCursor(want))
	if decodeError != nil {
		t.Fatal(decodeError)
	}
	if got != want {
		t.Fatalf("cursor = %+v, want %+v", got, want)
	}
}

func TestParsePageSizeBounds(t *testing.T) {
	if _, parseError := parsePageSize("101"); parseError == nil {
		t.Fatal("expected oversized page to fail")
	}
	if pageSize, parseError := parsePageSize(""); parseError != nil || pageSize != defaultPageSize {
		t.Fatalf("default page size = %d, %v", pageSize, parseError)
	}
}

func TestLogSearchRequiresBoundedTimeRange(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/logs?search=timeout", nil)
	if _, _, parseError := parseLogFilter(request); parseError == nil {
		t.Fatal("expected unbounded log search to fail")
	}

	from := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	to := from.Add(maxLogSearchRange)
	request = httptest.NewRequest(http.MethodGet, "/api/v1/logs?search=timeout&from="+from.Format(time.RFC3339)+"&to="+to.Format(time.RFC3339), nil)
	filter, _, parseError := parseLogFilter(request)
	if parseError != nil || filter.Search != "timeout" {
		t.Fatalf("filter = %#v, error = %v", filter, parseError)
	}

	tooWide := to.Add(time.Nanosecond)
	request = httptest.NewRequest(http.MethodGet, "/api/v1/logs?search=timeout&from="+from.Format(time.RFC3339Nano)+"&to="+tooWide.Format(time.RFC3339Nano), nil)
	if _, _, parseError = parseLogFilter(request); parseError == nil {
		t.Fatal("expected oversized log search range to fail")
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/logs?from="+to.Format(time.RFC3339)+"&to="+from.Format(time.RFC3339), nil)
	if _, _, parseError = parseLogFilter(request); parseError == nil {
		t.Fatal("expected reversed time range to fail")
	}
}
