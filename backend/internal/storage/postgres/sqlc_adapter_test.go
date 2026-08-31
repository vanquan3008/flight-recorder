package postgres

import (
	"testing"
	"time"
)

func TestSQLCScopeAndTime(t *testing.T) {
	project, environment, scopeError := sqlcScope("01900000-0000-7000-8000-000000000001", "01900000-0000-7000-8000-000000000002")
	if scopeError != nil || !project.Valid || !environment.Valid {
		t.Fatalf("sqlcScope() = (%v, %v, %v)", project, environment, scopeError)
	}
	if sqlcTime(time.Time{}).Valid {
		t.Fatal("zero time must be encoded as SQL NULL")
	}
	if !sqlcTime(time.Now()).Valid {
		t.Fatal("non-zero time must be encoded as a SQL timestamp")
	}
}
