package main

import (
	"context"
	"errors"
	"testing"
)

func TestRefreshRollupsDrainsBacklogLargerThanOneBatch(t *testing.T) {
	remaining := 2500
	passes := 0
	refresh := func(context.Context) (int, error) {
		passes++
		done := min(remaining, 1000)
		remaining -= done
		return done, nil
	}
	total, err := refreshRollups(context.Background(), refresh, 1000)
	if err != nil || total != 2500 || passes != 3 || remaining != 0 {
		t.Fatalf("total=%d passes=%d remaining=%d err=%v; a backlog over one batch must drain in one run", total, passes, remaining, err)
	}
}

func TestRefreshRollupsStopsOnErrorAndDeadline(t *testing.T) {
	failure := errors.New("clickhouse unavailable")
	total, err := refreshRollups(context.Background(), func(context.Context) (int, error) { return 1000, failure }, 1000)
	if !errors.Is(err, failure) || total != 1000 {
		t.Fatalf("total=%d err=%v; a failing pass must stop the loop and report the work done", total, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	passes := 0
	total, err = refreshRollups(ctx, func(context.Context) (int, error) {
		passes++
		cancel() // the deadline expires while the backlog is still full
		return 1000, nil
	}, 1000)
	if !errors.Is(err, context.Canceled) || passes != 1 || total != 1000 {
		t.Fatalf("passes=%d total=%d err=%v; the loop must stop when its context ends", passes, total, err)
	}
}
