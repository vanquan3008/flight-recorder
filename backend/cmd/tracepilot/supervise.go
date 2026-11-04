package main

import (
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/ingest/pipeline"
)

// component is one long-running part of the process, such as a server or a background worker.
type component struct {
	name     string
	critical bool
	run      func(context.Context) error
}

// superviseComponents runs every component until the context ends or one of them stops.
// Critical components failing will cancel the rest; auxiliary workers are automatically
// restarted with backoff to protect system availability.
func superviseComponents(ctx context.Context, logger *slog.Logger, components []component) error {
	group, groupContext := errgroup.WithContext(ctx)
	for _, part := range components {
		if part.critical {
			group.Go(func() error { return runComponent(groupContext, part.name, part.run) })
		} else {
			group.Go(func() error { return runResilientComponent(groupContext, logger, part.name, part.run) })
		}
	}
	return group.Wait()
}

func runResilientComponent(ctx context.Context, logger *slog.Logger, name string, run func(context.Context) error) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		err := run(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err == nil {
			logger.WarnContext(ctx, "auxiliary background worker exited unexpectedly; restarting", "worker", name)
		} else {
			logger.ErrorContext(ctx, "auxiliary background worker failed; restarting with backoff", "worker", name, "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(2 * time.Second):
		}
	}
}

// runComponent adapts a component's result for supervision:
//   - it returns nil when the parent context is cancelled;
//   - it returns an error when the component fails;
//   - it returns an error when the component exits unexpectedly;
//   - it treats HTTP server closure during shutdown as normal.
func runComponent(ctx context.Context, name string, run func(context.Context) error) error {
	err := run(ctx)
	if err != nil {
		if ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, http.ErrServerClosed)) {
			return nil
		}
		return errors.New(name + ": " + err.Error())
	}
	if ctx.Err() != nil {
		return nil
	}
	return errors.New(name + " stopped unexpectedly")
}

// ensureJWTSecret makes sure there is a signing secret. Tokens are signed with one secret shared by
// the API and the OTLP receiver; a random one is fine for a single local process, but configure one
// so tokens survive restarts and replicas agree.
func ensureJWTSecret(cfg *config.Config, logger *slog.Logger) error {
	if cfg.Auth.JWTSecret == "" {
		generated, secretError := auth.NewJWTSecret()
		if secretError != nil {
			return secretError
		}
		cfg.Auth.JWTSecret = hex.EncodeToString(generated)
		logger.Warn("TRACEPILOT_AUTH_JWT_SECRET is not set; using a random secret, so sign-ins and ingest tokens stop working when this process restarts")
	}
	if cfg.Auth.IngestJWTSecret == "" {
		cfg.Auth.IngestJWTSecret = cfg.Auth.JWTSecret
	}
	return nil
}

// drainPipelines stops ingestion first, then metrics, under one shared deadline.
func drainPipelines(ctx context.Context, events *pipeline.Pipeline, points *pipeline.MetricsPipeline) error {
	return errors.Join(events.Shutdown(ctx), points.Shutdown(ctx))
}
