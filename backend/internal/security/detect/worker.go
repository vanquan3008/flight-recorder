// Package detect runs durable security-rule evaluation outside ingestion.
package detect

import (
	"context"
	"log/slog"
	"time"
)

func Run(ctx context.Context, logger *slog.Logger, evaluate func(context.Context) (int, error), interval time.Duration) error {
	run := func() {
		if count, runError := evaluate(ctx); runError != nil {
			logger.Error("security detection failed", "error", runError)
		} else if count > 0 {
			logger.Info("security detections created", "count", count)
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
