package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const exportTimeout = 5 * time.Second

// LogExporter sends a Batch of log records to an OTLP endpoint.
// The concrete *Exporter type satisfies this interface, allowing unit tests
// to inject a mock without a live gRPC connection.
type LogExporter interface {
	Export(ctx context.Context, batch Batch) error
}

// AgentLoop manages the stateful tail → spool → export cycle for a single log
// source. Separating the loop body into Tick makes the core logic unit-testable
// independently of OS-level signal handling and ticker management.
type AgentLoop struct {
	// Config fields are set by the caller before the first Tick.
	Source          string
	CheckpointPath  string
	BufferDirectory string
	BufferBytes     int64
	BatchBytes      int
	BatchLines      int
	Exporter        LogExporter
	Metrics         *Metrics
	Logger          *slog.Logger

	// State managed across Tick calls.
	checkpoint  Checkpoint
	pending     Batch
	pendingCkpt Checkpoint
}

// NewAgentLoop creates an AgentLoop and loads the initial checkpoint from disk.
// Returns an error if the checkpoint file exists but cannot be parsed.
func NewAgentLoop(source, checkpointPath, bufferDirectory string, bufferBytes int64, batchBytes, batchLines int, exporter LogExporter, metrics *Metrics) (*AgentLoop, error) {
	return NewAgentLoopWithLogger(source, checkpointPath, bufferDirectory, bufferBytes, batchBytes, batchLines, exporter, metrics, slog.Default())
}

// NewAgentLoopWithLogger creates an AgentLoop with a custom logger.
// Use this when you need structured logging integrated with the parent process.
func NewAgentLoopWithLogger(source, checkpointPath, bufferDirectory string, bufferBytes int64, batchBytes, batchLines int, exporter LogExporter, metrics *Metrics, logger *slog.Logger) (*AgentLoop, error) {
	if exporter == nil {
		return nil, errors.New("agent exporter is required")
	}
	checkpoint, err := LoadCheckpoint(checkpointPath)
	if err != nil {
		return nil, err
	}
	if metrics == nil {
		metrics = NewMetrics()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &AgentLoop{
		Source:          source,
		CheckpointPath:  checkpointPath,
		BufferDirectory: bufferDirectory,
		BufferBytes:     bufferBytes,
		BatchBytes:      batchBytes,
		BatchLines:      batchLines,
		Exporter:        exporter,
		Metrics:         metrics,
		Logger:          logger,
		checkpoint:      checkpoint,
	}, nil
}

// Tick runs one iteration of the agent loop:
//  1. Drain one pending batch from the disk spool (if configured).
//  2. Read new lines from the source file when spool is clear and no batch is pending.
//  3. Export the pending batch to OTLP; on export failure, spill to the disk spool.
//
// Returns a non-nil error only for unrecoverable failures (checkpoint write failure,
// spool acknowledgement failure after a successful export). Transient errors — export
// timeouts, read errors, spool encode/enqueue failures — are logged as warnings and
// reported via Metrics so the loop can continue on the next Tick.
func (loop *AgentLoop) Tick(ctx context.Context) error {
	spoolBlocked := false

	// Step 1: drain one entry from the disk spool.
	if loop.BufferDirectory != "" {
		payload, acknowledge, dequeueError := Dequeue(loop.BufferDirectory)
		if dequeueError != nil {
			loop.Logger.Warn("spool dequeue failed", "source", loop.Source, "error", dequeueError)
			loop.Metrics.ObserveSpoolFailure()
			spoolBlocked = true
		} else if payload != nil {
			batch, decodeError := DecodeBatch(payload)
			if decodeError != nil {
				// Corrupt or unrecognisable spool entry: delete it and continue.
				loop.Logger.Warn("corrupt spool entry removed", "source", loop.Source, "error", decodeError)
				loop.Metrics.ObserveSpoolFailure()
				if ackErr := acknowledge(); ackErr != nil {
					return fmt.Errorf("acknowledge corrupt spool entry: %w", ackErr)
				}
			} else {
				exportCtx, cancel := context.WithTimeout(ctx, exportTimeout)
				exportError := loop.Exporter.Export(exportCtx, batch)
				cancel()
				if exportError != nil {
					loop.Logger.Warn("spool export failed", "source", loop.Source, "error", exportError)
					loop.Metrics.ObserveExportFailure()
					spoolBlocked = true
				} else {
					loop.Metrics.ObserveExport(len(batch))
					if ackErr := acknowledge(); ackErr != nil {
						return fmt.Errorf("acknowledge spool entry: %w", ackErr)
					}
				}
			}
		}
	}

	// Step 2: read new lines when the spool is clear and no batch is pending.
	if !spoolBlocked && len(loop.pending) == 0 {
		lines, next, skipped, readError := ReadLines(loop.Source, loop.checkpoint, loop.BatchBytes, loop.BatchBytes, loop.BatchLines)
		if readError != nil {
			loop.Logger.Warn("read lines failed", "source", loop.Source, "error", readError)
		} else {
			loop.Metrics.ObserveSkipped(skipped)
			if len(lines) == 0 && next != loop.checkpoint {
				loop.checkpoint = next
				if saveError := SaveCheckpoint(loop.CheckpointPath, loop.checkpoint); saveError != nil {
					return fmt.Errorf("save checkpoint after skipped lines: %w", saveError)
				}
			}
		}
		if readError == nil && len(lines) > 0 {
			loop.Metrics.ObserveRead(len(lines))
			loop.pending = NewBatch(lines)
			loop.pendingCkpt = next
		}
	}

	// Step 3: export the pending batch.
	if !spoolBlocked && len(loop.pending) > 0 {
		exportCtx, cancel := context.WithTimeout(ctx, exportTimeout)
		exportError := loop.Exporter.Export(exportCtx, loop.pending)
		cancel()
		if exportError == nil {
			loop.Metrics.ObserveExport(len(loop.pending))
			loop.checkpoint = loop.pendingCkpt
			if saveErr := SaveCheckpoint(loop.CheckpointPath, loop.checkpoint); saveErr != nil {
				return fmt.Errorf("save checkpoint: %w", saveErr)
			}
			loop.pending = nil
		} else {
			loop.Logger.Warn("export failed", "source", loop.Source, "error", exportError)
			loop.Metrics.ObserveExportFailure()
			if loop.BufferDirectory != "" {
				payload, encodeError := EncodeBatch(loop.pending)
				if encodeError != nil {
					loop.Logger.Warn("spool encode failed", "source", loop.Source, "error", encodeError)
					loop.Metrics.ObserveSpoolFailure()
				} else if enqueueError := Enqueue(loop.BufferDirectory, loop.BufferBytes, payload); enqueueError != nil {
					loop.Logger.Warn("spool enqueue failed", "source", loop.Source, "error", enqueueError)
					loop.Metrics.ObserveSpoolFailure()
				} else {
					loop.checkpoint = loop.pendingCkpt
					if saveErr := SaveCheckpoint(loop.CheckpointPath, loop.checkpoint); saveErr != nil {
						return fmt.Errorf("save checkpoint after spool: %w", saveErr)
					}
					loop.pending = nil
				}
			}
		}
	}

	return nil
}
