// Package intelligence runs observability incident detection outside the
// ingestion acknowledgement path.
package intelligence

import (
	"context"
	"log/slog"
	"time"
)

const DefaultInterval = time.Minute

func Run(ctx context.Context, logger *slog.Logger, evaluate func(context.Context) (int, error), interval time.Duration) error {
	if interval <= 0 {
		interval = DefaultInterval
	}
	run := func() {
		if count, err := evaluate(ctx); err != nil {
			logger.ErrorContext(ctx, "observability incident detection failed", "error", err)
		} else if count > 0 {
			logger.InfoContext(ctx, "observability incidents evaluated", "services", count)
		}
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			run()
		}
	}
}
