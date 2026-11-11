package migrations

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vanquan3008/tracepilot/db"
	migrationstate "github.com/vanquan3008/tracepilot/internal/storage/migrations"
)

func Inspect(ctx context.Context, databaseURL string) (migrationstate.Status, error) {
	list, err := db.List(db.PostgresMigrations)
	if err != nil {
		return migrationstate.Status{}, err
	}
	state := migrationstate.NewStatus(list, 0, false)
	connection, err := connect(ctx, databaseURL)
	if err != nil {
		return state, err
	}
	defer closeConnection(connection)
	var version int64
	err = connection.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&version, &state.Dirty)
	var pgError *pgconn.PgError
	if err != nil && !errors.Is(err, pgx.ErrNoRows) && !(errors.As(err, &pgError) && pgError.Code == "42P01") {
		return state, fmt.Errorf("read migration status: %w", err)
	}
	if version > 0 {
		state.Version = uint(version)
	}
	return migrationstate.NewStatus(list, state.Version, state.Dirty), nil
}

func connectionURL(databaseURL string) (*url.URL, error) {
	endpoint, err := url.Parse(databaseURL)
	if err != nil || (endpoint.Scheme != "postgres" && endpoint.Scheme != "postgresql" && endpoint.Scheme != "pgx5") {
		return nil, errors.New("migration database URL must use postgres or postgresql scheme")
	}
	endpoint.Scheme = "postgres"
	query := endpoint.Query()
	if !query.Has("connect_timeout") {
		query.Set("connect_timeout", "5")
	}
	query.Set("lock_timeout", "10000")
	endpoint.RawQuery = query.Encode()
	return endpoint, nil
}

func connect(ctx context.Context, databaseURL string) (*pgx.Conn, error) {
	endpoint, err := connectionURL(databaseURL)
	if err != nil {
		return nil, err
	}
	connection, err := pgx.Connect(ctx, endpoint.String())
	if err != nil {
		return nil, fmt.Errorf("connect migration database: %w", err)
	}
	return connection, nil
}

func closeConnection(connection *pgx.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = connection.Close(ctx)
}

// withLock also protects creation of the migration journal on a fresh database.
func withLock(ctx context.Context, databaseURL string, apply func() error) error {
	connection, err := connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer closeConnection(connection)
	if _, err := connection.Exec(ctx, "SELECT pg_advisory_lock($1)", int64(7_203_411_003)); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	return apply()
}
