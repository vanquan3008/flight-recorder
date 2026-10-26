package main

import (
	"testing"
	"time"
)

func TestPercentileUsesNearestRank(t *testing.T) {
	var sorted []time.Duration
	for millisecond := 1; millisecond <= 100; millisecond++ {
		sorted = append(sorted, time.Duration(millisecond)*time.Millisecond)
	}
	for p, want := range map[float64]time.Duration{50: 50 * time.Millisecond, 95: 95 * time.Millisecond, 99: 99 * time.Millisecond, 100: 100 * time.Millisecond, 0: time.Millisecond} {
		if got := percentile(sorted, p); got != want {
			t.Fatalf("p%.0f = %s, want %s", p, got, want)
		}
	}
	if percentile(nil, 99) != 0 || percentile(sorted[:1], 99) != time.Millisecond {
		t.Fatal("empty and single-sample inputs must not panic or invent values")
	}
}
