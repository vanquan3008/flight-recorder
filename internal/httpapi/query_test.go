package httpapi

import (
	"testing"
	"time"
)

func TestCursorRoundTrip(t *testing.T) {
	want := pageCursor{At: time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC), ID: "01900000-0000-7000-8000-000000000001"}
	got, decodeError := decodeCursor(encodeCursor(want))
	if decodeError != nil {
		t.Fatal(decodeError)
	}
	if got != want {
		t.Fatalf("cursor = %+v, want %+v", got, want)
	}
}

func TestParsePageSizeBounds(t *testing.T) {
	if _, parseError := parsePageSize("101"); parseError == nil {
		t.Fatal("expected oversized page to fail")
	}
	if pageSize, parseError := parsePageSize(""); parseError != nil || pageSize != defaultPageSize {
		t.Fatalf("default page size = %d, %v", pageSize, parseError)
	}
}
