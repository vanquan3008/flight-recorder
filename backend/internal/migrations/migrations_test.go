package migrations

import (
	"errors"
	"testing"

	"github.com/golang-migrate/migrate/v4"
)

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

func TestValidateCurrentVersion(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		version    uint
		dirty      bool
		versionErr error
		wantErr    bool
	}{
		{name: "current", version: CurrentVersion},
		{name: "uninitialized", versionErr: migrate.ErrNilVersion, wantErr: true},
		{name: "dirty", version: CurrentVersion, dirty: true, wantErr: true},
		{name: "behind", version: CurrentVersion - 1, wantErr: true},
		{name: "read failure", versionErr: errors.New("database unavailable"), wantErr: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateCurrentVersion(testCase.version, testCase.dirty, testCase.versionErr)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("validateCurrentVersion() error = %v, wantErr %t", err, testCase.wantErr)
			}
		})
	}
}
