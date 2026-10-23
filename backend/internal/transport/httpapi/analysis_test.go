package httpapi

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseAnalysisFilterDefaultsAndBounds(t *testing.T) {
	filter, err := parseAnalysisFilter(httptest.NewRequest("GET", "/api/v1/topology?service=checkout", nil))
	if err != nil {
		t.Fatal(err)
	}
	if filter.Service != "checkout" || filter.Limit != defaultPageSize || filter.To.Sub(filter.From) != 15*time.Minute {
		t.Fatalf("unexpected defaults: %#v", filter)
	}
	request := httptest.NewRequest("GET", "/api/v1/topology?from=2026-01-01T00:00:00Z&to=2026-02-02T00:00:00Z", nil)
	if _, err := parseAnalysisFilter(request); err == nil {
		t.Fatal("expected a range over 30 days to be rejected")
	}
}
