package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/vanquan3008/tracepilot/internal/ingest/pipeline"
)

const (
	minRestartBackoff = time.Second
	maxRestartBackoff = 30 * time.Second
	stableRunDuration = time.Minute
)

// component represents a long-running service or background worker.
type component struct {
	name     string
	critical bool
	run      func(context.Context) error
}

// superviseComponents manages all component lifecycles.
func superviseComponents(ctx context.Context, logger *slog.Logger, components []component) error {
	logger.Info("starting component supervisor", "components", len(components))

	group, groupCtx := errgroup.WithContext(ctx)
	started := time.Now()

	for _, comp := range components {
		c := comp
		group.Go(func() error {
			if c.critical {
				return runComponent(groupCtx, logger, c.name, c.run)
			}
			return runResilientComponent(groupCtx, logger, c.name, c.run)
		})
	}

	err := group.Wait()
	duration := time.Since(started)

	if err != nil {
		logger.Error("component supervisor stopped with error",
			"error", err,
			"uptime", duration.String(),
		)
		return err
	}

	logger.Info("component supervisor stopped",
		"uptime", duration.String(),
		"reason", "shutdown",
	)

	return nil
}

func runComponent(ctx context.Context, logger *slog.Logger, name string, run func(context.Context) error) error {
	log := logger.With("component", name, "critical", true)
	started := time.Now()
	log.Info("Component starting")

	err := run(ctx)
	duration := time.Since(started)
	if err == nil {
		if ctx.Err() != nil {
			log.Info("component stopped",
				"reason", "shutdown",
				"uptime", duration,
			)
			return nil
		}
		log.Error("critical component exited unexpectedly",
			"uptime", duration,
		)
		return fmt.Errorf("%s stopped unexpectedly", name)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, http.ErrServerClosed) {
		log.Info("component stopped",
			"reason", "shutdown",
			"error", err,
			"uptime", duration,
		)
		return nil
	}

	log.Error("critical component failed",
		"error", err,
		"uptime", duration,
	)
	return fmt.Errorf("%s: %w", name, err)
}

// runResilientComponent automatically restarts auxiliary workers.
func runResilientComponent(ctx context.Context, logger *slog.Logger, name string, run func(context.Context) error) error {
	log := logger.With("component", name, "critical", false)
	backoff := minRestartBackoff
	restarts := uint64(0)
	started := time.Now()
	log.Info("background worker starting")

	for ctx.Err() == nil {
		runStarted := time.Now()
		err := run(ctx)
		runDuration := time.Since(runStarted)
		if ctx.Err() != nil {
			break
		}
		if runDuration >= stableRunDuration {
			backoff = minRestartBackoff
		}

		restarts++

		if err != nil {
			log.Error("background worker failed",
				"error", err,
				"run_duration", runDuration.String(),
				"restart_count", restarts,
				"retry_in", backoff.String(),
			)
		} else {
			log.Warn("background worker exited unexpectedly",
				"run_duration", runDuration.String(),
				"restart_count", restarts,
				"retry_in", backoff.String(),
			)
		}

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			log.Info("background worker restart cancelled",
				"restart_count", restarts,
			)
			return nil
		case <-timer.C:
		}

		backoff = min(backoff*2, maxRestartBackoff)

		log.Info("background worker restarting",
			"restart_count", restarts,
		)
	}

	log.Info("background worker stopped",
		"reason", "shutdown",
		"uptime", time.Since(started).String(),
		"restart_count", restarts,
	)

	return nil
}

// drainPipelines gracefully stops ingestion pipelines concurrently.
func drainPipelines(ctx context.Context, logger *slog.Logger, events *pipeline.Pipeline, points *pipeline.MetricsPipeline) error {
	log := logger.With("component", "ingestion")
	started := time.Now()
	log.Info("pipeline drain starting")
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		eventErr  error
		metricErr error
	)

	shutdown := func(name string, stop func(context.Context) error, setErr func(error)) {
		defer wg.Done()

		pipelineLog := log.With("pipeline", name)
		pipelineStarted := time.Now()

		pipelineLog.Info("pipeline shutdown starting")

		err := stop(ctx)
		duration := time.Since(pipelineStarted).String()

		if err != nil {
			pipelineLog.Error("pipeline shutdown failed",
				"error", err,
				"duration", duration,
			)
			setErr(err)
			return
		}

		pipelineLog.Info("pipeline shutdown completed",
			"duration", duration,
		)
	}

	if events != nil {
		wg.Add(1)
		go shutdown("events", events.Shutdown, func(err error) {
			mu.Lock()
			eventErr = err
			mu.Unlock()
		})
	} else {
		log.Debug("event pipeline not initialized; skipping")
	}

	if points != nil {
		wg.Add(1)
		go shutdown("metrics", points.Shutdown, func(err error) {
			mu.Lock()
			metricErr = err
			mu.Unlock()
		})
	} else {
		log.Debug("metrics pipeline not initialized; skipping")
	}

	wg.Wait()

	err := errors.Join(eventErr, metricErr)
	duration := time.Since(started).String()

	if err != nil {
		log.Error("pipeline drain completed with errors",
			"error", err,
			"duration", duration,
			"context_error", ctx.Err(),
		)
		return err
	}

	log.Info("pipeline drain completed",
		"duration", duration,
	)

	return nil
}
