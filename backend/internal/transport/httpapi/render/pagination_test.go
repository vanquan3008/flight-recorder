package render_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"

	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

func TestCursorRoundTrip(t *testing.T) {
	want := render.PageCursor{At: time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC), ID: "01900000-0000-7000-8000-000000000001"}
	got, decodeError := render.DecodeCursor(render.EncodeCursor(want))
	if decodeError != nil {
		t.Fatal(decodeError)
	}
	if got != want {
		t.Fatalf("cursor = %+v, want %+v", got, want)
	}
}

func TestParsePageSizeBounds(t *testing.T) {
	if _, parseError := render.ParsePageSize("101"); parseError == nil {
		t.Fatal("expected oversized page to fail")
	}
	if pageSize, parseError := render.ParsePageSize(""); parseError != nil || pageSize != render.DefaultPageSize {
		t.Fatalf("default page size = %d, %v", pageSize, parseError)
	}
}

func TestParsePaginationRejectsMalformedCursor(t *testing.T) {
	valid := render.EncodeCursor(render.PageCursor{At: time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC), ID: "01900000-0000-7000-8000-000000000001"})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/logs?limit=10&cursor="+valid, nil)
	limit, cursor, parseError := render.ParsePagination(request, telemetry.ValidUUID)
	if parseError != nil || limit != 10 || cursor.ID != "01900000-0000-7000-8000-000000000001" {
		t.Fatalf("pagination = (%d, %#v, %v)", limit, cursor, parseError)
	}

	invalid := render.EncodeCursor(render.PageCursor{At: time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC), ID: "not-a-uuid"})
	request = httptest.NewRequest(http.MethodGet, "/api/v1/logs?cursor="+invalid, nil)
	if _, _, parseError = render.ParsePagination(request, telemetry.ValidUUID); parseError == nil {
		t.Fatal("expected malformed cursor ID to fail")
	}
}

func TestPageResponseOnlyReturnsCursorWhenMoreItemsExist(t *testing.T) {
	cursorOf := func(value int) render.PageCursor {
		return render.PageCursor{At: time.Unix(int64(value), 0), ID: strconv.Itoa(value)}
	}
	response := render.PageResponse([]int{1, 2}, 2, cursorOf)
	if _, hasCursor := response["next_cursor"]; hasCursor {
		t.Fatal("exactly one page must not return a cursor")
	}
	response = render.PageResponse([]int{1, 2, 3}, 2, cursorOf)
	if _, hasCursor := response["next_cursor"]; !hasCursor {
		t.Fatal("more items must return a cursor")
	}
	if data := response["data"].([]int); len(data) != 2 {
		t.Fatalf("page data length = %d", len(data))
	}
}

func TestPathIdentifierValidation(t *testing.T) {
	if !telemetry.ValidUUID("01900000-0000-7000-8000-000000000001") {
		t.Fatal("expected valid UUID")
	}
	for _, value := range []string{"", "not-a-uuid", "01900000-0000-7000-8000-00000000000z"} {
		if telemetry.ValidUUID(value) {
			t.Fatalf("accepted invalid UUID %q", value)
		}
	}
	if !render.ValidTraceID("0123456789abcdef0123456789abcdef") {
		t.Fatal("expected valid trace ID")
	}
	if render.ValidTraceID("0123456789abcdef") {
		t.Fatal("accepted short trace ID")
	}
	if !render.ValidSpanID("0123456789abcdef") {
		t.Fatal("expected valid span ID")
	}
	if render.ValidSpanID("0123456789abcdef0") {
		t.Fatal("accepted oversized span ID")
	}
}

func TestPathTraceIDLowersAndValidates(t *testing.T) {
	path := func(value string) *http.Request {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("traceID", value)
		return request
	}
	recorder := httptest.NewRecorder()
	if _, ok := render.PathTraceID(recorder, path("not-hex")); ok || recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
	if id, ok := render.PathTraceID(httptest.NewRecorder(), path("0123456789ABCDEF0123456789abcdef")); !ok || id != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("trace ID = %q, %t", id, ok)
	}
}

func TestQueryIDsAreOptionalLoweredAndValidated(t *testing.T) {
	trace, err := render.QueryTraceID(url.Values{"traceId": {" 0123456789ABCDEF0123456789abcdef "}})
	if err != nil || trace != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("trace = %q, %v", trace, err)
	}
	if span, err := render.QuerySpanID(url.Values{}); err != nil || span != "" {
		t.Fatalf("absent span = %q, %v", span, err)
	}
	if _, err := render.QuerySpanID(url.Values{"spanId": {"0123456789abcdef0"}}); err == nil {
		t.Fatal("accepted an oversized span ID")
	}
	if _, err := render.QueryTraceID(url.Values{"traceId": {"short"}}); err == nil {
		t.Fatal("accepted a short trace ID")
	}
}

func TestWindowStartEndsAtToThenTheCursorThenNow(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	to, before := now.Add(-2*time.Hour), now.Add(-30*time.Minute)
	for name, test := range map[string]struct{ to, before, want time.Time }{
		"no range ends now":           {time.Time{}, time.Time{}, now.Add(-time.Hour)},
		"only to ends at to":          {to, time.Time{}, to.Add(-time.Hour)},
		"a later page ends at cursor": {time.Time{}, before, before.Add(-time.Hour)},
		"to wins over the cursor":     {to, before, to.Add(-time.Hour)},
	} {
		if got := render.WindowStart(test.to, test.before, now); !got.Equal(test.want) {
			t.Errorf("%s: start = %s, want %s", name, got, test.want)
		}
	}
}

func TestParseTimeRangeReadsBothEndsAndChecksTheirOrder(t *testing.T) {
	from, to, err := render.ParseTimeRange(url.Values{"from": {"2026-01-01T00:00:00Z"}, "to": {"2026-01-02T00:00:00Z"}})
	if err != nil || to.Sub(from) != 24*time.Hour {
		t.Fatalf("range = %s..%s, %v", from, to, err)
	}
	if from, to, err = render.ParseTimeRange(url.Values{}); err != nil || !from.IsZero() || !to.IsZero() {
		t.Fatalf("empty range = %s..%s, %v", from, to, err)
	}
	for name, values := range map[string]url.Values{
		"bad from": {"from": {"yesterday"}},
		"bad to":   {"to": {"2026-13-01T00:00:00Z"}},
		"reversed": {"from": {"2026-01-02T00:00:00Z"}, "to": {"2026-01-01T00:00:00Z"}},
	} {
		if _, _, err := render.ParseTimeRange(values); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
