// Package lifecycle coordinates availability and shutdown of the databases.
package lifecycle

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"
)

type database interface {
	Ready(context.Context) error
	Close()
}

type Databases struct {
	metadata  database
	telemetry database
}

func NewDatabases(metadata, telemetry database) *Databases {
	return &Databases{metadata: metadata, telemetry: telemetry}
}

// Ready cancels the peer check as soon as either database fails.
func (databases *Databases) Ready(ctx context.Context) error {
	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		if err := databases.metadata.Ready(ctx); err != nil {
			return fmt.Errorf("PostgreSQL readiness: %w", err)
		}
		return nil
	})
	group.Go(func() error {
		if err := databases.telemetry.Ready(ctx); err != nil {
			return fmt.Errorf("ClickHouse readiness: %w", err)
		}
		return nil
	})
	return group.Wait()
}

func (databases *Databases) Close() {
	databases.telemetry.Close()
	databases.metadata.Close()
}
