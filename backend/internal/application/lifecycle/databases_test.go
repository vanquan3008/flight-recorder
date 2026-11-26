package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"
)

type databaseProbeFunc func(context.Context) error

func (probe databaseProbeFunc) Ready(ctx context.Context) error { return probe(ctx) }

func (databaseProbeFunc) Close() {}

func TestDatabaseReadinessRunsConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	started, release := make(chan struct{}, 2), make(chan struct{})
	probe := databaseProbeFunc(func(ctx context.Context) error {
		started <- struct{}{}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	done := make(chan error, 1)
	go func() { done <- NewDatabases(probe, probe).Ready(ctx) }()
	for range 2 {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("one database probe blocked the other")
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseReadinessCancelsPeerOnFailure(t *testing.T) {
	want := errors.New("database unavailable")
	for _, failed := range []string{"PostgreSQL", "ClickHouse"} {
		t.Run(failed, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			peerStarted := make(chan struct{})
			failure := databaseProbeFunc(func(context.Context) error { <-peerStarted; return want })
			peer := databaseProbeFunc(func(ctx context.Context) error {
				close(peerStarted)
				<-ctx.Done()
				return ctx.Err()
			})
			pg, ch := failure, peer
			if failed == "ClickHouse" {
				pg, ch = peer, failure
			}
			if err := NewDatabases(pg, ch).Ready(ctx); !errors.Is(err, want) {
				t.Fatalf("readiness lost the database failure: %v", err)
			}
		})
	}
}
