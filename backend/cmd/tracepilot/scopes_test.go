package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
	ingestotlp "github.com/vanquan3008/tracepilot/internal/ingest/otlp"
	"github.com/vanquan3008/tracepilot/internal/query"
	"github.com/vanquan3008/tracepilot/internal/storage/postgres"
)

type fakeScopeSource struct {
	projects     map[string]auth.Project
	environments map[string][]auth.Environment
	lookups      int
	failure      error
}

func (source *fakeScopeSource) GetProject(_ context.Context, reference string) (auth.Project, error) {
	source.lookups++
	if source.failure != nil {
		return auth.Project{}, source.failure
	}
	if project, found := source.projects[reference]; found {
		return project, nil
	}
	return auth.Project{}, auth.ErrProjectNotFound
}

func (source *fakeScopeSource) ListEnvironments(_ context.Context, projectID string) ([]auth.Environment, error) {
	return source.environments[projectID], nil
}

func (source *fakeScopeSource) ListScopes(context.Context) ([]auth.Scope, error) {
	var scopes []auth.Scope
	for id, project := range source.projects {
		for _, environment := range source.environments[id] {
			scopes = append(scopes, auth.Scope{Project: project, Environment: environment})
		}
	}
	return scopes, nil
}

func newTestCatalog() (*scopeCatalog, *fakeScopeSource, config.IngestionConfig) {
	base := config.IngestionConfig{ProjectID: "default", ProjectSlug: "default", ProjectName: "Default", EnvironmentID: "env-default", EnvironmentName: "development", BatchSize: 7}
	source := &fakeScopeSource{
		projects: map[string]auth.Project{"alpha": {ID: "alpha", Slug: "alpha", Name: "Alpha"}, "default": {ID: "default", Slug: "default", Name: "Default"}},
		environments: map[string][]auth.Environment{
			"alpha":   {{ID: "alpha-prod", ProjectID: "alpha", Name: "production"}, {ID: "alpha-stage", ProjectID: "alpha", Name: "staging"}},
			"default": {{ID: "env-default", ProjectID: "default", Name: "development"}},
		},
	}
	return newScopeCatalog(base, source), source, base
}

func TestConfiguredProjectResolvesWithoutALookup(t *testing.T) {
	catalog, source, base := newTestCatalog()
	for _, environment := range []string{"", "development", "env-default"} {
		scope, err := catalog.Resolve(context.Background(), "default", environment)
		if err != nil || scope.EnvironmentID != base.EnvironmentID {
			t.Fatalf("environment %q: scope %+v, error %v", environment, scope, err)
		}
	}
	if source.lookups != 0 {
		t.Fatalf("the configured project must not touch the database, got %d lookups", source.lookups)
	}
}

func TestOtherProjectsResolveByNameIDOrDefault(t *testing.T) {
	catalog, source, base := newTestCatalog()
	for environment, want := range map[string]string{"": "alpha-prod", "production": "alpha-prod", "staging": "alpha-stage", "alpha-stage": "alpha-stage"} {
		scope, err := catalog.Resolve(context.Background(), "alpha", environment)
		if err != nil || scope.EnvironmentID != want || scope.ProjectID != "alpha" || scope.ProjectSlug != "alpha" {
			t.Fatalf("environment %q: scope %+v, error %v", environment, scope, err)
		}
		if scope.BatchSize != base.BatchSize {
			t.Fatalf("a resolved scope must keep the configured limits, got batch size %d", scope.BatchSize)
		}
	}
	lookups := source.lookups
	if _, err := catalog.Resolve(context.Background(), "alpha", "staging"); err != nil || source.lookups != lookups {
		t.Fatalf("a repeated resolution must come from the cache (lookups %d -> %d, error %v)", lookups, source.lookups, err)
	}
}

func TestUnknownProjectsAndEnvironmentsAreRefused(t *testing.T) {
	catalog, _, _ := newTestCatalog()
	for _, testCase := range [][2]string{{"alpha", "qa"}, {"ghost", ""}, {"ghost", "production"}} {
		if _, err := catalog.Resolve(context.Background(), testCase[0], testCase[1]); !errors.Is(err, ingestotlp.ErrUnknownEnvironment) {
			t.Errorf("%v: error %v, want ErrUnknownEnvironment", testCase, err)
		}
	}
}

func TestLookupFailuresAreNotRemembered(t *testing.T) {
	catalog, source, _ := newTestCatalog()
	source.failure = errors.New("database unavailable")
	if _, err := catalog.Resolve(context.Background(), "alpha", ""); err == nil || errors.Is(err, ingestotlp.ErrUnknownEnvironment) {
		t.Fatalf("error %v, want the database failure", err)
	}
	source.failure = nil
	if scope, err := catalog.Resolve(context.Background(), "alpha", ""); err != nil || scope.ProjectID != "alpha" {
		t.Fatalf("after the database recovered: scope %+v, error %v", scope, err)
	}
}

func TestEveryEnvironmentIsListedForDetection(t *testing.T) {
	catalog, _, base := newTestCatalog()
	all, err := catalog.All(context.Background())
	if err != nil || len(all) != 3 {
		t.Fatalf("scopes %+v, error %v; want the default environment and alpha's two", all, err)
	}
	for _, scope := range all {
		if scope.ProjectID == "default" && scope.EnvironmentID != base.EnvironmentID {
			t.Fatalf("the configured project must keep its configured scope: %+v", scope)
		}
	}
}

func TestMixedBatchesAreStoredPerScopeAndRefusedWhole(t *testing.T) {
	catalog, _, _ := newTestCatalog()
	var stored []string
	store := storeEventsByScope(catalog, func(_ context.Context, scope config.IngestionConfig, events []telemetry.Event) error {
		stored = append(stored, scope.ProjectID+"/"+scope.EnvironmentName+":"+string(rune('0'+len(events))))
		return nil
	})
	events := []telemetry.Event{
		{ProjectID: "alpha", Environment: "production"}, {ProjectID: "alpha", Environment: "production"},
		{ProjectID: "alpha", Environment: "staging"}, {ProjectID: "default", Environment: "development"},
	}
	if err := store(context.Background(), events); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(sortedCopy(stored), " "); got != "alpha/production:2 alpha/staging:1 default/development:1" {
		t.Fatalf("stored groups = %q", got)
	}

	stored = nil
	err := store(context.Background(), append(events, telemetry.Event{ProjectID: "alpha", Environment: "deleted"}))
	if !errors.Is(err, telemetry.ErrInvalidEvent) || len(stored) != 0 {
		t.Fatalf("error %v, stored %v; an unresolvable scope must refuse the batch before any write, so the pipeline can isolate it", err, stored)
	}
}

func sortedCopy(values []string) []string {
	copied := append([]string(nil), values...)
	for i := range copied {
		for j := i + 1; j < len(copied); j++ {
			if copied[j] < copied[i] {
				copied[i], copied[j] = copied[j], copied[i]
			}
		}
	}
	return copied
}

func TestTwoProjectsKeepTheirTelemetryApartIntegration(t *testing.T) {
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("set TRACEPILOT_INTEGRATION_TEST=1 against a migrated disposable PostgreSQL database")
	}
	url := os.Getenv("TRACEPILOT_DATABASE_URL")
	if url == "" {
		t.Fatal("TRACEPILOT_DATABASE_URL is required")
	}
	ctx := context.Background()
	database, openError := postgres.Open(ctx, config.DatabaseConfig{URL: url, MaxConnections: 4, MaxConnectionLifetime: time.Minute, MaxConnectionIdleTime: time.Minute, HealthCheckPeriod: time.Minute})
	if openError != nil {
		t.Fatal(openError)
	}
	t.Cleanup(database.Close)

	run := strings.ToLower(time.Now().UTC().Format("150405.000000000"))
	run = strings.ReplaceAll(run, ".", "")
	base := config.IngestionConfig{
		ProjectID: telemetry.DeterministicID(time.Now(), "scope-base", run), ProjectSlug: "scope-base-" + run, ProjectName: "Scope base",
		EnvironmentID: telemetry.DeterministicID(time.Now(), "scope-base-env", run), EnvironmentName: "test", ScrubFreeText: true,
	}
	if err := database.EnsureIngestionScope(ctx, base); err != nil {
		t.Fatal(err)
	}
	actor := auth.Actor{}
	one, _, oneError := database.CreateProject(ctx, auth.CreateProjectParams{Slug: "scope-one-" + run, Name: "One"}, "", 0, actor)
	two, _, twoError := database.CreateProject(ctx, auth.CreateProjectParams{Slug: "scope-two-" + run, Name: "Two", EnvironmentName: "staging"}, "", 0, actor)
	if oneError != nil || twoError != nil {
		t.Fatal(oneError, twoError)
	}
	cleanup, poolError := pgxpool.New(ctx, url)
	if poolError != nil {
		t.Fatal(poolError)
	}
	t.Cleanup(func() {
		defer cleanup.Close()
		// Partitioned telemetry has no cascading foreign keys, so each table is cleared before the project.
		for _, id := range []string{one.ID, two.ID, base.ProjectID} {
			for _, table := range []string{"incidents", "spans", "events", "metric_data_points"} {
				if _, err := cleanup.Exec(ctx, "DELETE FROM "+table+" WHERE project_id = $1", id); err != nil {
					t.Error(err)
				}
			}
			if _, err := cleanup.Exec(ctx, "DELETE FROM projects WHERE id = $1", id); err != nil {
				t.Error(err)
			}
		}
	})

	catalog := newScopeCatalog(base, database)
	store := storeEventsByScope(catalog, database.StoreEvents)
	at := time.Now().UTC().Truncate(time.Second)
	span := func(project auth.Project, environment, service, spanID string) telemetry.Event {
		return telemetry.Event{
			ID: telemetry.DeterministicID(at, "scope-span", project.ID, spanID), ProjectID: project.ID, Environment: environment, ServiceName: service,
			Type: telemetry.EventRPC, Signal: telemetry.SignalTrace, Timestamp: at, Duration: 5 * time.Millisecond, Status: "OK", Operation: "work",
			SpanKind: 2, TraceID: strings.Repeat(spanID, 16), SpanID: strings.Repeat(spanID, 8),
			Attributes: map[string]any{}, ResourceAttributes: map[string]any{}, ScopeAttributes: map[string]any{}, RecordAttributes: map[string]any{},
		}
	}
	// One batch mixes both projects, as the shared pipeline's batches do.
	if err := store(ctx, []telemetry.Event{span(one, "production", "checkout", "a"), span(two, "staging", "billing", "b")}); err != nil {
		t.Fatal(err)
	}

	for project, want := range map[auth.Project]string{one: "checkout", two: "billing"} {
		scope, err := catalog.Resolve(ctx, project.ID, "")
		if err != nil {
			t.Fatal(err)
		}
		traces, err := database.ListTraces(ctx, project.ID, query.TraceFilter{EnvironmentID: scope.EnvironmentID, From: at.Add(-time.Hour), To: at.Add(time.Hour), Limit: 10})
		if err != nil || len(traces) != 1 || traces[0].RootService != want {
			t.Fatalf("project %s: traces %+v, error %v; want exactly its own %q trace", project.Slug, traces, err, want)
		}
	}
	all, err := catalog.All(ctx)
	found := 0
	for _, scope := range all {
		if scope.ProjectID == one.ID || scope.ProjectID == two.ID {
			found++
		}
	}
	if err != nil || found != 2 {
		t.Fatalf("detection must see both new projects, found %d (error %v)", found, err)
	}
}
