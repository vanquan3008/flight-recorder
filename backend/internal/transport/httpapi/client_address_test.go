package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
)

func serverTrusting(proxies ...string) *Server {
	configuration := config.Config{
		HTTP:     config.HTTPConfig{ManagementMaxBodyBytes: 1024, RateLimitRequests: 1, RateLimitWindow: time.Minute, TrustedProxies: proxies},
		Database: config.DatabaseConfig{ReadinessTimeout: time.Second},
		App:      config.AppConfig{ShutdownTimeout: time.Second},
	}
	return New(configuration, slog.New(slog.NewTextHandler(io.Discard, nil)), func(context.Context) error { return nil }, metrics.New("test", "test", "test"), nil, nil)
}

func TestClientAddressTrustsForwardedForOnlyFromConfiguredProxies(t *testing.T) {
	for _, test := range []struct {
		name       string
		trusted    []string
		remote     string
		forwarded  []string
		wantClient string
	}{
		{"no proxies configured ignores the header", nil, "10.0.0.5:4000", []string{"203.0.113.9"}, "10.0.0.5"},
		{"header from an unlisted peer is spoofable and ignored", []string{"10.0.0.0/8"}, "198.51.100.7:4000", []string{"203.0.113.9"}, "198.51.100.7"},
		{"trusted proxy reports the client it saw", []string{"10.0.0.0/8"}, "10.0.0.5:4000", []string{"203.0.113.9"}, "203.0.113.9"},
		{"a client-supplied leftmost value is never used", []string{"10.0.0.0/8"}, "10.0.0.5:4000", []string{"1.2.3.4, 203.0.113.9"}, "203.0.113.9"},
		{"trusted hops on the right are skipped", []string{"10.0.0.0/8"}, "10.0.0.5:4000", []string{"203.0.113.9, 10.0.0.7"}, "203.0.113.9"},
		{"repeated headers form one chain", []string{"10.0.0.0/8"}, "10.0.0.5:4000", []string{"1.2.3.4", "203.0.113.9"}, "203.0.113.9"},
		{"a malformed chain falls back to the peer", []string{"10.0.0.0/8"}, "10.0.0.5:4000", []string{"203.0.113.9, not-an-ip"}, "10.0.0.5"},
		{"only trusted hops falls back to the peer", []string{"10.0.0.0/8"}, "10.0.0.5:4000", []string{"10.0.0.6"}, "10.0.0.5"},
		{"a single address is a valid entry", []string{"192.0.2.1"}, "192.0.2.1:4000", []string{"203.0.113.9"}, "203.0.113.9"},
		{"IPv4-mapped IPv6 peers match IPv4 entries", []string{"10.0.0.0/8"}, "[::ffff:10.0.0.5]:4000", []string{"203.0.113.9"}, "203.0.113.9"},
		{"IPv6 clients are keyed by address", []string{"fd00::/8"}, "[fd00::1]:4000", []string{"2001:db8::9"}, "2001:db8::9"},
		{"a missing remote address is labelled", nil, "", nil, "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.RemoteAddr = test.remote
			for _, value := range test.forwarded {
				request.Header.Add("X-Forwarded-For", value)
			}
			if got := serverTrusting(test.trusted...).clientAddress(request); got != test.wantClient {
				t.Fatalf("client = %q, want %q", got, test.wantClient)
			}
		})
	}
}

func TestRateLimitSeparatesClientsBehindATrustedProxy(t *testing.T) {
	server := serverTrusting("10.0.0.0/8")
	send := func(client string) int {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
		request.RemoteAddr = "10.0.0.5:4000"
		request.Header.Set("X-Forwarded-For", client)
		recorder := httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(recorder, request)
		return recorder.Code
	}
	if first := send("203.0.113.1"); first == http.StatusTooManyRequests {
		t.Fatal("first request was limited")
	}
	if send("203.0.113.1") != http.StatusTooManyRequests {
		t.Fatal("the same client was not limited")
	}
	if send("203.0.113.2") == http.StatusTooManyRequests {
		t.Fatal("a different client behind the same proxy shared the first client's bucket")
	}
}

func TestRateLimitCannotBeEvadedByForgingForwardedFor(t *testing.T) {
	server := serverTrusting() // nothing trusted
	var limited int
	for index := range 4 {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
		request.RemoteAddr = "198.51.100.7:4000"
		request.Header.Set("X-Forwarded-For", "203.0.113."+string(rune('1'+index)))
		recorder := httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(recorder, request)
		if recorder.Code == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited != 3 {
		t.Fatalf("limited %d of 4 requests, want 3: rotating the header must not mint new buckets", limited)
	}
}
