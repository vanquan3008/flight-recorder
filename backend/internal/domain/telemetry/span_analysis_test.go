package telemetry

import "testing"

func TestSpanEndpointMatchesTheSQLDefinition(t *testing.T) {
	for _, test := range []struct {
		name       string
		attributes map[string]any
		spanName   string
		want       string
	}{
		{"route with stable method", map[string]any{"http.route": "/orders/{id}", "http.request.method": "GET", "http.method": "POST"}, "x", "GET /orders/{id}"},
		{"route with legacy method", map[string]any{"http.route": "/orders", "http.request.method": "", "http.method": "POST"}, "x", "POST /orders"},
		{"route without method", map[string]any{"http.route": "/health"}, "x", "/health"},
		{"rpc with service", map[string]any{"rpc.service": "Checkout", "rpc.method": "Pay"}, "x", "Checkout/Pay"},
		{"rpc without service", map[string]any{"rpc.method": "Pay"}, "x", "Pay"},
		{"empty route falls back", map[string]any{"http.route": ""}, "work", "work"},
		{"no name", nil, "", "(unnamed)"},
	} {
		if got := SpanEndpoint(test.attributes, test.spanName); got != test.want {
			t.Errorf("%s: SpanEndpoint = %q, want %q", test.name, got, test.want)
		}
	}
}

func TestSpanRequestFailedOnErrorStatusOrHTTP5xx(t *testing.T) {
	for _, test := range []struct {
		name       string
		status     string
		attributes map[string]any
		want       bool
	}{
		{"error status", "ERROR", nil, true},
		{"stable integer 503", "UNSET", map[string]any{"http.response.status_code": int64(503)}, true},
		{"legacy string 500", "OK", map[string]any{"http.status_code": "500"}, true},
		{"stable code wins over legacy", "UNSET", map[string]any{"http.response.status_code": int64(404), "http.status_code": int64(500)}, false},
		{"empty stable code falls back", "UNSET", map[string]any{"http.response.status_code": "", "http.status_code": int64(502)}, true},
		{"client error", "UNSET", map[string]any{"http.response.status_code": int64(499)}, false},
		{"not three digits", "UNSET", map[string]any{"http.response.status_code": "5000"}, false},
	} {
		if got := SpanRequestFailed(test.status, test.attributes); got != test.want {
			t.Errorf("%s: SpanRequestFailed = %t, want %t", test.name, got, test.want)
		}
	}
}

func TestSpanDependencyNamesDatabasesQueuesAndExternalCalls(t *testing.T) {
	for _, test := range []struct {
		name       string
		attributes map[string]any
		kind       int32
		wantType   string
		wantName   string
	}{
		{"database namespace", map[string]any{"db.system": "postgresql", "db.namespace": "orders", "server.address": "db1"}, 3, "database", "orders"},
		{"database address", map[string]any{"db.system.name": "redis", "server.address": "cache"}, 1, "database", "cache"},
		{"database system only", map[string]any{"db.system": "sqlite"}, 1, "database", "sqlite"},
		{"queue destination", map[string]any{"messaging.system": "kafka", "messaging.destination.name": "orders"}, 4, "queue", "orders"},
		{"queue system only", map[string]any{"messaging.system": "rabbitmq"}, 5, "queue", "rabbitmq"},
		{"client peer service", map[string]any{"peer.service": "payments", "server.address": "10.0.0.1"}, 3, "external", "payments"},
		{"client url", map[string]any{"url.full": "https://example.com/x"}, 3, "external", "https://example.com/x"},
		{"anonymous client", map[string]any{}, 3, "external", ""},
		{"server span", map[string]any{"http.route": "/x"}, 2, "", ""},
	} {
		gotType, gotName := SpanDependency(test.attributes, test.kind)
		if gotType != test.wantType || gotName != test.wantName {
			t.Errorf("%s: SpanDependency = %q, %q; want %q, %q", test.name, gotType, gotName, test.wantType, test.wantName)
		}
	}
}

func TestMetricSeriesIDIgnoresValuesAndTimes(t *testing.T) {
	monotonic := true
	first := MetricDataPoint{ServiceName: "api", MetricName: "requests", MetricType: MetricTypeSum, IsMonotonic: &monotonic,
		Temporality: "cumulative", Attributes: map[string]any{"route": "/a"}}
	later := first
	value := 42.0
	later.Value, later.ID = &value, "another point"
	if first.SeriesID() != later.SeriesID() {
		t.Fatal("a new value started a new series")
	}
	empty, missing := first, first
	empty.ResourceAttributes, missing.ResourceAttributes = map[string]any{}, nil
	if empty.SeriesID() != missing.SeriesID() {
		t.Fatal("nil and empty resource attributes named different series")
	}
	other := first
	other.Attributes = map[string]any{"route": "/b"}
	if other.SeriesID() == first.SeriesID() || len(first.SeriesID()) != 32 {
		t.Fatalf("series IDs %q and %q must differ and be 32 hex characters", first.SeriesID(), other.SeriesID())
	}
}
