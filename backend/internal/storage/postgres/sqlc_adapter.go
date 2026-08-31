package postgres

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func sqlcScope(projectID, environmentID string) (pgtype.UUID, pgtype.UUID, error) {
	project, projectError := sqlcUUID(projectID)
	if projectError != nil {
		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("project ID: %w", projectError)
	}
	environment, environmentError := sqlcUUID(environmentID)
	if environmentError != nil {
		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("environment ID: %w", environmentError)
	}
	return project, environment, nil
}

func sqlcUUID(value string) (pgtype.UUID, error) {
	var identifier pgtype.UUID
	if scanError := identifier.Scan(value); scanError != nil {
		return pgtype.UUID{}, scanError
	}
	return identifier, nil
}

func sqlcTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: !value.IsZero()}
}
