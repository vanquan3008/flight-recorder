package signals

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

// markInvalidEvents tags PostgreSQL errors caused by row content, so the
// ingestion pipeline can isolate the offending events instead of retrying the
// whole batch. Class 22 is "data exception"; 23502/23514 are NOT NULL and CHECK
// violations. Connection, lock and unique errors stay temporary.
func markInvalidEvents(err error) error {
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		return err
	}
	if strings.HasPrefix(postgresError.Code, "22") || postgresError.Code == "23502" || postgresError.Code == "23514" {
		return fmt.Errorf("%w: %w", telemetry.ErrInvalidEvent, err)
	}
	return err
}
