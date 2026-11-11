package migrations

import (
	"context"
	"errors"
	"testing"
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
