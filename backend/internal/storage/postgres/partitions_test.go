package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type partitionTransaction struct {
	pgx.Tx
	createError error
	statements  []string
}

func (transaction *partitionTransaction) Exec(_ context.Context, query string, _ ...any) (pgconn.CommandTag, error) {
	transaction.statements = append(transaction.statements, query)
	if strings.HasPrefix(query, "CREATE TABLE") {
		return pgconn.CommandTag{}, transaction.createError
	}
	return pgconn.CommandTag{}, nil
}

func TestCreatePartitionOnlyToleratesDefaultPartitionConflict(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		fail bool
	}{
		{"created", nil, false},
		{"default contains rows", &pgconn.PgError{Code: "23514"}, false},
		{"permission denied", &pgconn.PgError{Code: "42501"}, true},
		{"disk full", &pgconn.PgError{Code: "53100"}, true},
		{"connection lost", errors.New("connection lost"), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			transaction := &partitionTransaction{createError: test.err}
			err := createDailyPartition(context.Background(), transaction, "events", time.Now().UTC())
			if test.fail {
				if !errors.Is(err, test.err) {
					t.Fatalf("creation failure was hidden: %v", err)
				}
			} else if err != nil || transaction.statements[len(transaction.statements)-1] != "RELEASE SAVEPOINT create_partition" {
				t.Fatalf("partition/savepoint not completed: error=%v statements=%v", err, transaction.statements)
			}
		})
	}
}
