package telemetry

import (
	"strings"
	"testing"
	"time"
)

func TestDeterministicIDIsStableOrderedUUIDv7(t *testing.T) {
	firstTime := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	first := DeterministicID(firstTime, "trace", "span")
	retry := DeterministicID(firstTime, "trace", "span")
	later := DeterministicID(firstTime.Add(time.Millisecond), "trace", "span")
	if first != retry {
		t.Fatalf("retry ID changed: %q != %q", first, retry)
	}
	if first[14] != '7' || !strings.ContainsRune("89ab", rune(first[19])) {
		t.Fatalf("ID is not RFC 9562 version 7 with the RFC variant: %q", first)
	}
	if first >= later {
		t.Fatalf("IDs are not timestamp ordered: %q >= %q", first, later)
	}
}
