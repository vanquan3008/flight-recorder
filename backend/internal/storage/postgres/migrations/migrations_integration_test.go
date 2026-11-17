package migrations

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vanquan3008/tracepilot/db"
)

func migrationDatabase(t *testing.T) (context.Context, string, *pgx.Conn) {
	t.Helper()
	if os.Getenv("TRACEPILOT_INTEGRATION_TEST") != "1" {
		t.Skip("requires a disposable PostgreSQL server with CREATE DATABASE permission")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	admin, err := pgx.Connect(ctx, os.Getenv("TRACEPILOT_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeConnection(admin) })
	name := fmt.Sprintf("tracepilot_migration_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+quoted+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	endpoint, err := url.Parse(os.Getenv("TRACEPILOT_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	endpoint.Path = "/" + name
	connection, err := pgx.Connect(ctx, endpoint.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeConnection(connection) })
	return ctx, endpoint.String(), connection
}

func TestMigrationFreshInstallAndRerunPreservesDataIntegration(t *testing.T) {
	ctx, endpoint, connection := migrationDatabase(t)
	// The source is embedded and works without the repository's SQL directory.
	t.Chdir(t.TempDir())
	state, err := Inspect(ctx, endpoint)
	if err != nil || state.Version != 0 || len(state.Pending) == 0 {
		t.Fatalf("fresh status: %+v, %v", state, err)
	}
	var untouched bool
	if err := connection.QueryRow(ctx, "SELECT to_regclass('schema_migrations') IS NULL").Scan(&untouched); err != nil || !untouched {
		t.Fatalf("status created metadata: %t, %v", untouched, err)
	}
	if _, err := Run(ctx, endpoint, "up", 0); err != nil {
		t.Fatal(err)
	}
	// Reapplying the baseline must leave existing telemetry untouched.
	_, err = connection.Exec(ctx, `INSERT INTO projects(id, slug, name) VALUES ('00000000-0000-0000-0000-000000000001', 'upgrade', 'Upgrade');
INSERT INTO environments(id, project_id, name) VALUES ('00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001', 'test');
INSERT INTO spans(event_id, project_id, environment_id, trace_id, span_id, span_name, span_kind, started_at, ended_at, status_code, service_name)
VALUES ('00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000002',
'trace', 'span', 'GET /upgrade', 2, now(), now(), 'OK', 'api')`)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := Run(ctx, endpoint, "up", 0); err != nil {
			t.Fatal(err)
		}
	}
	var spans int
	if err := connection.QueryRow(ctx, "SELECT count(*) FROM spans").Scan(&spans); err != nil || spans != 1 {
		t.Fatalf("rerun changed data: spans=%d error=%v", spans, err)
	}
	state, err = Inspect(ctx, endpoint)
	if err != nil || state.Version != state.Target || state.Dirty || len(state.Pending) != 0 {
		t.Fatalf("upgraded status: %+v, %v", state, err)
	}
}

func TestMigrationFailureLeavesDirtyAndBlocksRetryIntegration(t *testing.T) {
	ctx, endpoint, connection := migrationDatabase(t)
	if _, err := connection.Exec(ctx, "CREATE TABLE users (id integer)"); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, endpoint, "up", 0); err == nil {
		t.Fatal("conflicting schema was accepted")
	}
	state, err := Inspect(ctx, endpoint)
	list, listErr := db.List(db.PostgresMigrations)
	var identityVersion uint
	for _, migration := range list {
		if strings.HasSuffix(migration.File, "_init.up.sql") {
			identityVersion = migration.Version
		}
	}
	if err != nil || listErr != nil || identityVersion == 0 || !state.Dirty || state.Version != identityVersion {
		t.Fatalf("failed migration was not dirty: %+v, %v", state, err)
	}
	var rolledBack bool
	if err := connection.QueryRow(ctx, "SELECT to_regclass('projects') IS NULL").Scan(&rolledBack); err != nil || !rolledBack {
		t.Fatalf("failed baseline left partially created tables: %v", err)
	}
	if _, err := Run(ctx, endpoint, "up", 0); err == nil || !strings.Contains(err.Error(), "Dirty") {
		t.Fatalf("dirty migration retried: %v", err)
	}
}

func TestMigrationLockHonorsCancellationAndReleasesIntegration(t *testing.T) {
	ctx, endpoint, connection := migrationDatabase(t)
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		finished <- withLock(ctx, endpoint, Options{}.withDefaults(), func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	defer func() {
		close(release)
		if err := <-finished; err != nil {
			t.Error(err)
		}
		if err := withLock(ctx, endpoint, Options{}.withDefaults(), func() error { return nil }); err != nil {
			t.Errorf("migration lock was not released: %v", err)
		}
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	wait, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	called := false
	err := withLock(wait, endpoint, Options{}.withDefaults(), func() error { called = true; return nil })
	if err == nil || called {
		t.Fatalf("concurrent migration bypassed lock: called=%t error=%v", called, err)
	}
	mutation, cancelMutation := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancelMutation()
	if _, err := Run(mutation, endpoint, "up", 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("runner did not wait for the migration lock: %v", err)
	}
	state, err := Inspect(ctx, endpoint)
	if err != nil || state.Version != 0 {
		t.Fatalf("read-only status waited for a mutation lock: %+v, %v", state, err)
	}
	var untouched bool
	if err := connection.QueryRow(ctx, "SELECT to_regclass('schema_migrations') IS NULL").Scan(&untouched); err != nil || !untouched {
		t.Fatalf("waiting runner created metadata before acquiring its lock: %t, %v", untouched, err)
	}
}

func TestConcurrentMigrationsSerializeOnFreshDatabaseIntegration(t *testing.T) {
	ctx, endpoint, _ := migrationDatabase(t)
	start, finished := make(chan struct{}), make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := Run(ctx, endpoint, "up", 0)
			finished <- err
		}()
	}
	close(start)
	for range 2 {
		if err := <-finished; err != nil {
			t.Error(err)
		}
	}
	state, err := Inspect(ctx, endpoint)
	if err != nil || state.Dirty || state.Version != state.Target {
		t.Fatalf("concurrent migration corrupted history: %+v, %v", state, err)
	}
}

func TestMigrationRefusesNewerSchemaIntegration(t *testing.T) {
	ctx, endpoint, connection := migrationDatabase(t)
	if _, err := Run(ctx, endpoint, "up", 0); err != nil {
		t.Fatal(err)
	}
	expected, err := CurrentVersion()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, "UPDATE schema_migrations SET version = $1", expected+1); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, endpoint, "up", 0); err == nil {
		t.Fatal("older binary accepted a newer schema")
	}
}

func TestBaselineRollbackIsRejectedWithoutChangingSchemaIntegration(t *testing.T) {
	ctx, endpoint, connection := migrationDatabase(t)
	if _, err := Run(ctx, endpoint, "up", 0); err != nil {
		t.Fatal(err)
	}
	version, err := CurrentVersion()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, endpoint, "down", int(version)); err == nil || !strings.Contains(err.Error(), "no rollback SQL") {
		t.Fatalf("baseline rollback was not rejected: %v", err)
	}
	state, err := Inspect(ctx, endpoint)
	if err != nil || state.Version != state.Target || state.Dirty {
		t.Fatalf("rejected rollback changed the journal: %+v, %v", state, err)
	}
	var exists bool
	if err := connection.QueryRow(ctx, "SELECT to_regclass('projects') IS NOT NULL").Scan(&exists); err != nil || !exists {
		t.Fatalf("rejected rollback changed the schema: %v", err)
	}
	if _, err := Run(ctx, endpoint, "up", 0); err != nil {
		t.Fatal(err)
	}
}
