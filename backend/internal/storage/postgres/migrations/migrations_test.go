package migrations

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunRejectsUnsafeOperationBeforeConnecting(t *testing.T) {
	if _, runError := Run(context.Background(), "postgres://unused", "drop", 0); runError == nil {
		t.Fatal("expected unsupported operation to fail")
	}
}

func TestRunHonorsCancellationBeforeConnecting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Run(ctx, "postgres://unused", "up", 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled migration attempted a connection: %v", err)
	}
}

func TestRunRequiresExplicitDownSteps(t *testing.T) {
	if _, runError := Run(context.Background(), "postgres://unused", "down", 0); runError == nil {
		t.Fatal("expected down without steps to fail")
	}
}

func TestConnectionURLUsesConfiguredTimeouts(t *testing.T) {
	endpoint, err := connectionURL("postgres://user@db/tracepilot", Options{ConnectTimeout: 2500 * time.Millisecond, LockTimeout: 30 * time.Second}.withDefaults())
	if err != nil {
		t.Fatal(err)
	}
	query := endpoint.Query()
	if query.Get("connect_timeout") != "3" || query.Get("lock_timeout") != "30000" {
		t.Fatalf("query = %v, want connect_timeout=3 and lock_timeout=30000", query)
	}
	explicit, err := connectionURL("postgres://user@db/tracepilot?connect_timeout=9", Options{}.withDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if explicit.Query().Get("connect_timeout") != "9" || explicit.Query().Get("lock_timeout") != "10000" {
		t.Fatalf("query = %v; the URL value and default lock timeout must win", explicit.Query())
	}
}
