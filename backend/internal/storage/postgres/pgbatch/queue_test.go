package pgbatch

import (
	"context"
	"errors"
	"testing"
)

func TestQueueRecordsWritesAndRejectsReads(t *testing.T) {
	queue := &Queue{}
	if _, err := queue.Exec(context.Background(), "INSERT INTO t VALUES ($1)", 1); err != nil {
		t.Fatal(err)
	}
	if queue.Len() != 1 {
		t.Fatalf("queued %d statements, want 1", queue.Len())
	}
	if _, err := queue.Query(context.Background(), "SELECT 1"); !errors.Is(err, errReadInBatch) {
		t.Fatalf("Query error = %v", err)
	}
	if err := queue.QueryRow(context.Background(), "SELECT 1").Scan(); !errors.Is(err, errReadInBatch) {
		t.Fatalf("QueryRow error = %v", err)
	}
}
