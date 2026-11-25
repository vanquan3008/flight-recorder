// Package access is the API's security boundary. It authenticates every request against the scope
// its route declares, selects the project and environment, and bounds how many requests use the
// database at once, before any handler runs.
package access

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/observability/metrics"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/api"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

const (
	// signInSlots bounds concurrent sign-ins, which have their own pool.
	signInSlots = 16
	// gateWait is how long a request waits for a free slot before it is refused.
	// It absorbs a burst without letting a backlog grow.
	gateWait = 500 * time.Millisecond
)

// Store is what admission reads on every request.
type Store interface {
	GetUser(context.Context, string) (auth.User, error)
	GetProject(context.Context, string) (auth.Project, error)
	MemberRole(ctx context.Context, projectID, userID string) (string, error)
	ListEnvironments(context.Context, string) ([]auth.Environment, error)
}

// Config holds the admission settings. The configuration loader validates and defaults them.
type Config struct {
	TokenSecret   []byte
	TokenIssuer   string
	TokenAudience string
	// The configured project keeps its configured environment without a lookup.
	DefaultProjectID     string
	DefaultEnvironmentID string

	IdentityCacheTTL   time.Duration
	IdentityCacheLimit int
	ScopeCacheTTL      time.Duration
	ScopeCacheLimit    int

	MaxConcurrentQueries    int
	MaxConcurrentLogQueries int
	RequestTimeout          time.Duration
}

// Guard admits API requests. It owns the short-lived project, role, and environment caches and the
// bounded slot pools.
type Guard struct {
	store   Store
	metrics *metrics.Registry
	errors  api.Errors

	tokenSecret          []byte
	tokenIssuer          string
	tokenAudience        string
	defaultProjectID     string
	defaultEnvironmentID string

	projects     *ttlCache[auth.Project]
	roles        *ttlCache[string]
	environments *ttlCache[[]auth.Environment]

	queries        gate
	logQueries     gate
	signIns        gate
	requestTimeout time.Duration
}

// gate is a bounded pool of slots. Log reads and sign-ins have their own, so they never wait behind
// slow PostgreSQL queries.
type gate struct {
	name  string
	slots chan struct{}
}

func New(configuration Config, store Store, logger *slog.Logger, registry *metrics.Registry) *Guard {
	return &Guard{
		store:   store,
		metrics: registry,
		errors:  api.Errors{Logger: logger},

		tokenSecret:          configuration.TokenSecret,
		tokenIssuer:          configuration.TokenIssuer,
		tokenAudience:        configuration.TokenAudience,
		defaultProjectID:     configuration.DefaultProjectID,
		defaultEnvironmentID: configuration.DefaultEnvironmentID,

		projects:     newTTLCache[auth.Project](configuration.IdentityCacheTTL, configuration.IdentityCacheLimit),
		roles:        newTTLCache[string](configuration.IdentityCacheTTL, configuration.IdentityCacheLimit),
		environments: newTTLCache[[]auth.Environment](configuration.ScopeCacheTTL, configuration.ScopeCacheLimit),

		queries:        newGate("query", configuration.MaxConcurrentQueries),
		logQueries:     newGate("logs", configuration.MaxConcurrentLogQueries),
		signIns:        newGate("auth", signInSlots),
		requestTimeout: configuration.RequestTimeout,
	}
}

func newGate(name string, size int) gate {
	return gate{name: name, slots: make(chan struct{}, size)}
}

// Admit wraps a route's handler in the admission its route declares.
func (guard *Guard) Admit(route api.Route) http.HandlerFunc {
	switch {
	case route.Public:
		return guard.public(route.Handler)
	case route.Streaming:
		return guard.streaming(route.Scope, route.Handler)
	case route.LogQuery:
		return guard.scoped(route.Scope, guard.logQueries, route.Handler)
	default:
		return guard.scoped(route.Scope, guard.queries, route.Handler)
	}
}

// scoped authenticates a request against its route's scope. An authenticated request then takes one
// of a bounded number of slots and runs under a deadline, so API load can neither take every database
// connection from ingestion nor hold one indefinitely.
func (guard *Guard) scoped(scope string, pool gate, handle api.Handler) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		// The deadline starts before authentication: the user and environment lookups also wait for a
		// pooled connection, and without it a request could wait there past every proxy timeout.
		queryContext, cancel := context.WithTimeout(request.Context(), guard.requestTimeout)
		defer cancel()
		request = request.WithContext(queryContext)
		principal, _, ok := guard.authenticate(responseWriter, request, scope)
		if !ok || !guard.selectEnvironment(responseWriter, request, &principal) {
			return
		}
		release, admitted := guard.admit(request.Context(), pool)
		if !admitted {
			writeOverloaded(responseWriter)
			return
		}
		defer release()
		handle(responseWriter, request, principal)
	}
}

// streaming authenticates a long-lived connection without holding a slot or the request deadline.
// The connection ends when the access token expires or the caller loses access.
func (guard *Guard) streaming(scope string, handle api.Handler) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		principal, claims, ok := guard.authenticate(responseWriter, request, scope)
		if !ok {
			return
		}
		// Token verification tolerates clock skew. A stream outlives any single request, so it starts
		// only while the token is strictly unexpired.
		if !time.Now().Before(claims.ExpiresAt) {
			render.WriteAPIError(responseWriter, http.StatusUnauthorized, "TOKEN_EXPIRED", "authentication required")
			return
		}
		if !guard.selectEnvironment(responseWriter, request, &principal) {
			return
		}
		streamContext, cancel := guard.watchStream(request.Context(), principal, claims)
		defer cancel()
		handle(responseWriter, request.WithContext(streamContext), principal)
	}
}

// public serves a route that needs no credentials under its own slot pool, so sign-ins cannot
// exhaust the pool and are never starved by telemetry queries.
func (guard *Guard) public(handle api.Handler) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		release, admitted := guard.admit(request.Context(), guard.signIns)
		if !admitted {
			writeOverloaded(responseWriter)
			return
		}
		defer release()
		queryContext, cancel := context.WithTimeout(request.Context(), guard.requestTimeout)
		defer cancel()
		handle(responseWriter, request.WithContext(queryContext), auth.Principal{})
	}
}

// admit takes a slot from pool, waiting briefly for one. The returned func frees the slot.
func (guard *Guard) admit(ctx context.Context, pool gate) (func(), bool) {
	start := time.Now()
	timer := time.NewTimer(gateWait)
	defer timer.Stop()
	select {
	case pool.slots <- struct{}{}:
		guard.metrics.ObserveGateWait(pool.name, time.Since(start))
		guard.metrics.IncGateActive(pool.name)
		return func() {
			<-pool.slots
			guard.metrics.DecGateActive(pool.name)
		}, true
	case <-timer.C:
		guard.metrics.ObserveGateWait(pool.name, time.Since(start))
		return nil, false
	case <-ctx.Done():
		guard.metrics.ObserveGateWait(pool.name, time.Since(start))
		return nil, false
	}
}

func writeOverloaded(responseWriter http.ResponseWriter) {
	responseWriter.Header().Set("Retry-After", "1")
	render.WriteAPIError(responseWriter, http.StatusServiceUnavailable, "OVERLOADED", "too many requests are running; retry shortly")
}

// ForgetProject drops a renamed project from the cache.
func (guard *Guard) ForgetProject(projectID string) { guard.projects.forget(projectID) }

// ForgetRole drops a member whose role changed from the cache.
func (guard *Guard) ForgetRole(projectID, userID string) {
	guard.roles.forget(roleCacheKey(projectID, userID))
}

// ForgetEnvironments drops a project's environment list after one is added.
func (guard *Guard) ForgetEnvironments(projectID string) { guard.environments.forget(projectID) }
