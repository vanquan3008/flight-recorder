package migrations

import "testing"

func TestRunRejectsUnsafeOperationBeforeConnecting(t *testing.T) {
	if _, runError := Run("postgres://unused", ".", "drop", 0); runError == nil {
		t.Fatal("expected unsupported operation to fail")
	}
}

func TestRunRequiresExplicitDownSteps(t *testing.T) {
	if _, runError := Run("postgres://unused", ".", "down", 0); runError == nil {
		t.Fatal("expected down without steps to fail")
	}
}
