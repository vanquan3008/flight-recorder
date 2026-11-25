package otlp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/vanquan3008/tracepilot/internal/ingest/otlp/normalize/normalizetest"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type guardFixture struct {
	receiver *Receiver
	clock    time.Time
	lookups  int
	result   func(key string) (auth.Principal, error)
}

// newGuardFixture builds a receiver whose authenticate counts database lookups
// and whose guard clock the test controls.
func newGuardFixture(limit int) *guardFixture {
	fixture := &guardFixture{clock: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}
	fixture.result = func(string) (auth.Principal, error) { return auth.Principal{}, auth.ErrUnauthorized }
	configuration := normalizetest.Config()
	configuration.AuthFailureLimit = limit
	configuration.AuthFailureWindow = time.Minute
	fixture.receiver = NewReceiver(configuration, nil, nil, metrics.New("test", "test", "test"), func(_ context.Context, key string) (auth.Principal, error) {
		fixture.lookups++
		return fixture.result(key)
	})
	fixture.receiver.guard.now = func() time.Time { return fixture.clock }
	return fixture
}

func (fixture *guardFixture) authorize(key, client string) error {
	_, authorizeError := fixture.receiver.authorizeScope(context.Background(), key, "", client)
	return authorizeError
}

func TestAuthGuardRemembersRejectedKeysBriefly(t *testing.T) {
	fixture := newGuardFixture(1000)
	for index := range 5 {
		if err := fixture.authorize("junk", "client-"+string(rune('a'+index))); !errors.Is(err, auth.ErrUnauthorized) {
			t.Fatalf("attempt %d: %v", index, err)
		}
	}
	if fixture.lookups != 1 {
		t.Fatalf("a retried bad key reached the database %d times, want 1", fixture.lookups)
	}
	fixture.clock = fixture.clock.Add(badKeyTTL + time.Second)
	_ = fixture.authorize("junk", "client-z")
	if fixture.lookups != 2 {
		t.Fatalf("the rejection was not forgotten after its TTL: %d lookups", fixture.lookups)
	}
}

func TestAuthGuardThrottlesClientPastItsFailureBudget(t *testing.T) {
	fixture := newGuardFixture(3)
	for _, key := range []string{"k1", "k2", "k3"} {
		_ = fixture.authorize(key, "10.0.0.1")
	}
	if fixture.lookups != 3 {
		t.Fatalf("lookups before the limit = %d", fixture.lookups)
	}
	err := fixture.authorize("k4", "10.0.0.1")
	var throttled *throttledError
	if !errors.Is(err, ErrThrottled) || !errors.As(err, &throttled) || throttled.retryAfter <= 0 {
		t.Fatalf("over-budget client was not throttled: %v", err)
	}
	if fixture.lookups != 3 {
		t.Fatalf("a throttled request still reached the database (%d lookups)", fixture.lookups)
	}
	if err := fixture.authorize("k5", "10.0.0.2"); !errors.Is(err, auth.ErrUnauthorized) || errors.Is(err, ErrThrottled) {
		t.Fatalf("another client was affected: %v", err)
	}
	fixture.clock = fixture.clock.Add(time.Minute)
	if err := fixture.authorize("k6", "10.0.0.1"); errors.Is(err, ErrThrottled) {
		t.Fatal("client stayed throttled after the window ended")
	}
}

func TestAuthGuardNeverCachesAcceptedKeys(t *testing.T) {
	fixture := newGuardFixture(1000)
	fixture.result = func(string) (auth.Principal, error) {
		return auth.Principal{ProjectID: fixture.receiver.projectID, Scopes: []string{"ingest"}}, nil
	}
	if err := fixture.authorize("good", "client"); err != nil {
		t.Fatal(err)
	}
	fixture.result = func(string) (auth.Principal, error) { return auth.Principal{}, auth.ErrUnauthorized }
	if err := fixture.authorize("good", "client"); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("a revoked key was still accepted: %v", err)
	}
	if fixture.lookups != 2 {
		t.Fatalf("lookups = %d, want one per request", fixture.lookups)
	}
}

func TestAuthGuardIgnoresOutagesAndScopeFailuresForKeyMemory(t *testing.T) {
	fixture := newGuardFixture(1000)
	fixture.result = func(string) (auth.Principal, error) { return auth.Principal{}, errors.New("database down") }
	for range 5 {
		if err := fixture.authorize("key", "client"); errors.Is(err, auth.ErrUnauthorized) || errors.Is(err, ErrThrottled) {
			t.Fatalf("an outage was reported as %v", err)
		}
	}
	if fixture.lookups != 5 {
		t.Fatalf("an outage must not be cached: %d lookups", fixture.lookups)
	}

	fixture = newGuardFixture(1000)
	fixture.result = func(string) (auth.Principal, error) {
		return auth.Principal{ProjectID: fixture.receiver.projectID, Scopes: []string{"read"}}, nil
	}
	for range 2 {
		if err := fixture.authorize("read-only", "client"); !errors.Is(err, auth.ErrUnauthorized) {
			t.Fatalf("a key without ingest scope was accepted: %v", err)
		}
	}
	if fixture.lookups != 2 {
		t.Fatalf("a valid key without the scope was remembered as bad: %d lookups", fixture.lookups)
	}
}

func TestAuthGuardDisabledWithoutALimit(t *testing.T) {
	fixture := &guardFixture{}
	configuration := normalizetest.Config()
	fixture.receiver = NewReceiver(configuration, nil, nil, metrics.New("test", "test", "test"), func(context.Context, string) (auth.Principal, error) {
		fixture.lookups++
		return auth.Principal{}, auth.ErrUnauthorized
	})
	for range 50 {
		_ = fixture.authorize("junk", "client")
	}
	if fixture.receiver.guard != nil || fixture.lookups != 50 {
		t.Fatalf("guard=%v lookups=%d", fixture.receiver.guard, fixture.lookups)
	}
}

func TestAuthGuardTablesStayBounded(t *testing.T) {
	guard := newAuthGuard(5, time.Minute)
	for index := range 3 * maxGuardEntries {
		client := "client-" + strings.Repeat("x", index%7) + string(rune('A'+index%26)) + time.Duration(index).String()
		guard.failed(client, client, true)
	}
	if len(guard.clients) > maxGuardEntries || len(guard.badKeys) > maxGuardEntries {
		t.Fatalf("tables grew past the bound: %d clients, %d keys", len(guard.clients), len(guard.badKeys))
	}
}

func TestOTLPHTTPReturns429WhenClientKeepsFailingAuthentication(t *testing.T) {
	configuration := normalizetest.Config()
	configuration.AuthFailureLimit = 2
	configuration.AuthFailureWindow = time.Minute
	lookups := 0
	receiver := NewReceiver(configuration, nil, nil, metrics.New("test", "test", "test"), func(context.Context, string) (auth.Principal, error) {
		lookups++
		return auth.Principal{}, auth.ErrUnauthorized
	})
	server := NewServer(config.Config{Ingestion: configuration}, receiver)
	send := func(key string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader("{}"))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+key)
		request.RemoteAddr = "203.0.113.9:40000"
		response := httptest.NewRecorder()
		server.httpServer.Handler.ServeHTTP(response, request)
		return response
	}
	for _, key := range []string{"one", "two"} {
		if response := send(key); response.Code != http.StatusUnauthorized {
			t.Fatalf("%s: status %d", key, response.Code)
		}
	}
	response := send("three")
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") == "" {
		t.Fatalf("status=%d Retry-After=%q", response.Code, response.Header().Get("Retry-After"))
	}
	if lookups != 2 {
		t.Fatalf("lookups = %d; the throttled request must not reach the database", lookups)
	}
}

func TestOTLPGRPCReportsResourceExhaustedWhenThrottled(t *testing.T) {
	fixture := newGuardFixture(1)
	if code := status.Code(grpcAuthError(fixture.receiver)); code != codes.Unauthenticated {
		t.Fatalf("first failure = %v", code)
	}
	if code := status.Code(grpcAuthError(fixture.receiver)); code != codes.ResourceExhausted {
		t.Fatalf("throttled call = %v", code)
	}
}

func TestAuthGuardCountsFailuresPerClientBehindATrustedProxy(t *testing.T) {
	fixture := newGuardFixture(2)
	fixture.receiver.WithTrustedProxies([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
	attacker := func(key string) error {
		return fixture.authorize(key, fixture.receiver.clientAddress("10.0.0.5:4000", []string{"203.0.113.9"}))
	}
	bystander := func(key string) error {
		return fixture.authorize(key, fixture.receiver.clientAddress("10.0.0.5:4000", []string{"198.51.100.2"}))
	}
	_ = attacker("a1")
	_ = attacker("a2")
	if err := attacker("a3"); !errors.Is(err, ErrThrottled) {
		t.Fatalf("the failing client was not throttled: %v", err)
	}
	if err := bystander("b1"); errors.Is(err, ErrThrottled) {
		t.Fatal("a client sharing the proxy was throttled for someone else's failures")
	}
	// Without trusted proxies a forged header changes nothing: the peer is the identity.
	plain := newGuardFixture(2)
	if got := plain.receiver.clientAddress("10.0.0.5:4000", []string{"203.0.113.9"}); got != "10.0.0.5" {
		t.Fatalf("an untrusted peer chose its own identity: %q", got)
	}
}

// grpcAuthError is the error authorizeGRPC reports; the scoped context it also returns is not under test here.
func grpcAuthError(receiver *Receiver) error {
	_, authError := receiver.authorizeGRPC(context.Background())
	return authError
}
