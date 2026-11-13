package main

import (
	"strings"
	"testing"
	"time"
)

func TestPercentileUsesNearestRank(t *testing.T) {
	sorted := []time.Duration{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for p, want := range map[float64]time.Duration{50: 5, 95: 10, 99: 10, 10: 1} {
		if got := percentile(sorted, p); got != want {
			t.Errorf("p%.0f = %d, want %d", p, got, want)
		}
	}
	if percentile(nil, 95) != 0 {
		t.Error("empty input must give zero")
	}
}

func TestRejectedRecordsReadsStringAndNumberCounts(t *testing.T) {
	cases := map[string]int64{
		`{}`: 0,
		`{"partialSuccess":{"rejectedSpans":"3","errorMessage":"bad span"}}`: 3,
		`{"partialSuccess":{"rejectedLogRecords":2}}`:                        2,
		`not json`: 0,
	}
	for payload, want := range cases {
		if got := rejectedRecords([]byte(payload)); got != want {
			t.Errorf("rejectedRecords(%s) = %d, want %d", payload, got, want)
		}
	}
}

func TestPickReadPathFallsBackWithoutATrace(t *testing.T) {
	total := 0
	for _, endpoint := range readEndpoints {
		total += endpoint.weight
	}
	detailRoll := total - 1
	if got, _ := pickReadPath(detailRoll, ""); got != readEndpoints[0].path {
		t.Fatalf("detail without trace = %q", got)
	}
	if got, endpoint := pickReadPath(detailRoll, "abc"); got != "/api/v1/traces/abc" || endpoint != "/api/v1/traces/{id}" {
		t.Fatalf("detail with trace = %q reported as %q", got, endpoint)
	}
	if got, _ := pickReadPath(0, "abc"); got != readEndpoints[0].path {
		t.Fatalf("first roll = %q", got)
	}
}

func TestTraceExportKeepsTheRequestedSpanCount(t *testing.T) {
	export, exported := traceExport(20, 0, time.Now())
	if len(exported) != 3 {
		t.Fatalf("trace IDs = %d, want 3 for 20 spans of %d", len(exported), spansPerTrace)
	}
	spans := 0
	for _, resource := range export["resourceSpans"].([]any) {
		for _, scope := range resource.(map[string]any)["scopeSpans"].([]any) {
			spans += len(scope.(map[string]any)["spans"].([]any))
		}
	}
	if spans != 20 {
		t.Fatalf("spans = %d, want 20", spans)
	}
}

func TestParseMetricsKeepsOnlyReportedSeries(t *testing.T) {
	body := strings.NewReader(`# HELP x
tracepilot_ingestion_queue_depth{pipeline="events"} 12
tracepilot_db_pool_connections{state="acquired"} 3
tracepilot_http_requests_total{code="200"} 99
tracepilot_ingestion_retries_total 1
`)
	values := parseMetrics(body)
	if len(values) != 3 || values[`tracepilot_ingestion_queue_depth{pipeline="events"}`] != 12 || values["tracepilot_ingestion_retries_total"] != 1 {
		t.Fatalf("values = %v", values)
	}
}

func TestParseOptionsRequiresAWorkload(t *testing.T) {
	if _, err := parseOptions(nil); err == nil {
		t.Fatal("a run without any rate was accepted")
	}
	settings, err := parseOptions([]string{"-spans-per-second", "100", "-run", "fixed"})
	if err != nil || settings.spansPerSecond != 100 || settings.runLabel != "fixed" {
		t.Fatalf("settings = %+v, err = %v", settings, err)
	}
}
