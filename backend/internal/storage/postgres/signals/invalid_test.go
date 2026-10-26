package signals

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vanquan3008/tracepilot/internal/domain/telemetry"
)

func TestMarkInvalidEventsSeparatesContentFromTemporaryErrors(t *testing.T) {
	for code, invalid := range map[string]bool{
		"22P05": true,  // untranslatable character, e.g. U+0000 in jsonb
		"22021": true,  // invalid byte sequence for the encoding
		"23502": true,  // NOT NULL violation
		"23514": true,  // CHECK violation
		"23505": false, // unique violation: concurrent writer, retry is safe
		"40P01": false, // deadlock
		"57P01": false, // admin shutdown
		"08006": false, // connection failure
	} {
		err := markInvalidEvents(fmt.Errorf("insert event batch: %w", &pgconn.PgError{Code: code}))
		if got := errors.Is(err, telemetry.ErrInvalidEvent); got != invalid {
			t.Fatalf("code %s invalid = %t, want %t", code, got, invalid)
		}
	}
	if markInvalidEvents(nil) != nil {
		t.Fatal("success became an error")
	}
	if plain := errors.New("pool closed"); markInvalidEvents(plain) != plain {
		t.Fatal("non-PostgreSQL error was rewritten")
	}
}
