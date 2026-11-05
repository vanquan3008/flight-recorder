package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	ingestotlp "github.com/vanquan3008/tracepilot/internal/ingest/otlp"
)

// scopeSource is the part of the database the catalog reads.
type scopeSource interface {
	GetProject(context.Context, string) (auth.Project, error)
	ListEnvironments(context.Context, string) ([]auth.Environment, error)
	ListScopes(context.Context) ([]auth.Scope, error)
}

const (
	scopeCacheTTL = 30 * time.Second
	// scopeCacheLimit bounds the cache; senders choose the environment text, so it must not grow without limit.
	scopeCacheLimit = 1024
)

type cachedScope struct {
	scope    config.IngestionConfig
	err      error
	loadedAt time.Time
}

// scopeCatalog turns "this project, this environment" into the ingestion scope telemetry is stored under.
// The configured project keeps its configured environment without a lookup; every other project resolves
// through the database, briefly cached.
type scopeCatalog struct {
	base   config.IngestionConfig
	source scopeSource
	now    func() time.Time

	mu    sync.Mutex
	cache map[string]cachedScope
}

func newScopeCatalog(base config.IngestionConfig, source scopeSource) *scopeCatalog {
	return &scopeCatalog{base: base, source: source, now: time.Now, cache: make(map[string]cachedScope)}
}

// scopeFor copies the configured limits and swaps in the project and environment.
func (catalog *scopeCatalog) scopeFor(project auth.Project, environment auth.Environment) config.IngestionConfig {
	scope := catalog.base
	scope.ProjectID, scope.ProjectSlug, scope.ProjectName = project.ID, project.Slug, project.Name
	scope.EnvironmentID, scope.EnvironmentName = environment.ID, environment.Name
	return scope
}

// Resolve finds the scope for a project and an environment named by ID or name. An empty
// environment means the project's first. An environment the project lacks reports
// ingestotlp.ErrUnknownEnvironment.
func (catalog *scopeCatalog) Resolve(ctx context.Context, projectID, environment string) (config.IngestionConfig, error) {
	base := catalog.base
	if projectID == base.ProjectID && (environment == "" || environment == base.EnvironmentName || environment == base.EnvironmentID) {
		return base, nil
	}
	key := projectID + "\x00" + environment
	now := catalog.now()
	catalog.mu.Lock()
	entry, cached := catalog.cache[key]
	catalog.mu.Unlock()
	if cached && now.Sub(entry.loadedAt) < scopeCacheTTL {
		return entry.scope, entry.err
	}
	scope, resolveError := catalog.load(ctx, projectID, environment)
	// A lookup that failed for a reason other than "not there" is not remembered: the database may be back in a moment.
	if resolveError == nil || errors.Is(resolveError, ingestotlp.ErrUnknownEnvironment) {
		catalog.mu.Lock()
		if len(catalog.cache) >= scopeCacheLimit {
			catalog.cache = make(map[string]cachedScope)
		}
		catalog.cache[key] = cachedScope{scope: scope, err: resolveError, loadedAt: now}
		catalog.mu.Unlock()
	}
	return scope, resolveError
}

func (catalog *scopeCatalog) load(ctx context.Context, projectID, environment string) (config.IngestionConfig, error) {
	project, projectError := catalog.source.GetProject(ctx, projectID)
	if errors.Is(projectError, auth.ErrProjectNotFound) {
		return config.IngestionConfig{}, ingestotlp.ErrUnknownEnvironment
	}
	if projectError != nil {
		return config.IngestionConfig{}, projectError
	}
	environments, listError := catalog.source.ListEnvironments(ctx, project.ID)
	if listError != nil {
		return config.IngestionConfig{}, listError
	}
	for _, candidate := range environments {
		if environment == "" || candidate.ID == environment || candidate.Name == environment {
			return catalog.scopeFor(project, candidate), nil
		}
	}
	return config.IngestionConfig{}, ingestotlp.ErrUnknownEnvironment
}

// All lists every project environment, for work that runs in each of them.
func (catalog *scopeCatalog) All(ctx context.Context) ([]config.IngestionConfig, error) {
	scopes, listError := catalog.source.ListScopes(ctx)
	if listError != nil {
		return nil, listError
	}
	all := make([]config.IngestionConfig, 0, len(scopes))
	for _, scope := range scopes {
		if scope.Project.ID == catalog.base.ProjectID && scope.Environment.ID == catalog.base.EnvironmentID {
			all = append(all, catalog.base)
			continue
		}
		all = append(all, catalog.scopeFor(scope.Project, scope.Environment))
	}
	return all, nil
}

type scopeKey struct{ project, environment string }

// storeByScope stores a batch that may mix projects, one group per scope. Every scope is
// resolved before anything is written, and one that no longer exists marks the batch invalid so
// the pipeline isolates those items instead of retrying forever.
func storeByScope[T any](catalog *scopeCatalog, scopeOf func(T) scopeKey, store func(context.Context, config.IngestionConfig, []T) error) func(context.Context, []T) error {
	return func(ctx context.Context, items []T) error {
		groups := make(map[scopeKey][]T)
		for _, item := range items {
			key := scopeOf(item)
			groups[key] = append(groups[key], item)
		}
		scopes := make(map[scopeKey]config.IngestionConfig, len(groups))
		for key := range groups {
			scope, resolveError := catalog.Resolve(ctx, key.project, key.environment)
			if errors.Is(resolveError, ingestotlp.ErrUnknownEnvironment) {
				return errors.Join(telemetry.ErrInvalidEvent, resolveError)
			}
			if resolveError != nil {
				return resolveError
			}
			scopes[key] = scope
		}
		var storeError error
		for key, group := range groups {
			storeError = errors.Join(storeError, store(ctx, scopes[key], group))
		}
		return storeError
	}
}

// storeEventsByScope groups events by the project and environment name they carry.
func storeEventsByScope(catalog *scopeCatalog, store func(context.Context, config.IngestionConfig, []telemetry.Event) error) func(context.Context, []telemetry.Event) error {
	return storeByScope(catalog, func(event telemetry.Event) scopeKey { return scopeKey{event.ProjectID, event.Environment} }, store)
}

// storeMetricsByScope groups metric data points, which name their environment by ID.
func storeMetricsByScope(catalog *scopeCatalog, store func(context.Context, config.IngestionConfig, []telemetry.MetricDataPoint) error) func(context.Context, []telemetry.MetricDataPoint) error {
	return storeByScope(catalog, func(point telemetry.MetricDataPoint) scopeKey { return scopeKey{point.ProjectID, point.EnvironmentID} }, store)
}
