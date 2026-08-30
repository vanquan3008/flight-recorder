package management

import (
	"net/http"
	"net/http/httptest"
	"strconv"
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

func TestParseIncidentFilter(t *testing.T) {
	from := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/incidents?service=checkout&severity=critical&status=resolved&from="+from.Format(time.RFC3339), nil)
	filter, _, parseError := parseIncidentFilter(request)
	if parseError != nil {
		t.Fatal(parseError)
	}
	if filter.Service != "checkout" || filter.Severity != "CRITICAL" || filter.Status != "RESOLVED" || !filter.From.Equal(from) {
		t.Fatalf("filter = %#v", filter)
	}
	invalid := httptest.NewRequest(http.MethodGet, "/api/v1/incidents?status=closed", nil)
	if _, _, parseError := parseIncidentFilter(invalid); parseError == nil {
		t.Fatal("expected invalid incident status to fail")
	}
}

func TestParseFiltersRejectMalformedCursorIDs(t *testing.T) {
	cursor := encodeCursor(pageCursor{At: time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC), ID: "not-an-id"})
	if _, _, parseError := parseTraceFilter(httptest.NewRequest(http.MethodGet, "/api/v1/traces?cursor="+cursor, nil)); parseError == nil {
		t.Fatal("expected malformed trace cursor to fail")
	}
	if _, _, parseError := parseLogFilter(httptest.NewRequest(http.MethodGet, "/api/v1/logs?cursor="+cursor, nil)); parseError == nil {
		t.Fatal("expected malformed log cursor to fail")
	}
	if _, _, parseError := parseIncidentFilter(httptest.NewRequest(http.MethodGet, "/api/v1/incidents?cursor="+cursor, nil)); parseError == nil {
		t.Fatal("expected malformed incident cursor to fail")
	}
}

func TestPageResponseOnlyReturnsCursorWhenMoreItemsExist(t *testing.T) {
	items := []int{1, 2}
	response := pageResponse(items, 2, func(value int) pageCursor { return pageCursor{At: time.Unix(int64(value), 0), ID: strconv.Itoa(value)} })
	if _, hasCursor := response["next_cursor"]; hasCursor {
		t.Fatal("exactly one page must not return a cursor")
	}
	response = pageResponse([]int{1, 2, 3}, 2, func(value int) pageCursor { return pageCursor{At: time.Unix(int64(value), 0), ID: strconv.Itoa(value)} })
	if _, hasCursor := response["next_cursor"]; !hasCursor {
		t.Fatal("more items must return a cursor")
	}
	if data := response["data"].([]int); len(data) != 2 {
		t.Fatalf("page data length = %d", len(data))
	}
}

func TestPathIdentifierValidation(t *testing.T) {
	if !validUUID("01900000-0000-7000-8000-000000000001") {
		t.Fatal("expected valid UUID")
	}
	for _, value := range []string{"", "not-a-uuid", "01900000-0000-7000-8000-00000000000z"} {
		if validUUID(value) {
			t.Fatalf("accepted invalid UUID %q", value)
		}
	}
	if !validTraceID("0123456789abcdef0123456789abcdef") {
		t.Fatal("expected valid trace ID")
	}
	if validTraceID("0123456789abcdef") {
		t.Fatal("accepted short trace ID")
	}
}
