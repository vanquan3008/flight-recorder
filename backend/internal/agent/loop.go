package agent

import (
	"context"
	"fmt"
	"os"
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

	// State managed across Tick calls.
	checkpoint  Checkpoint
	pending     Batch
	pendingCkpt Checkpoint
}

// NewAgentLoop creates an AgentLoop and loads the initial checkpoint from disk.
// Returns an error if the checkpoint file exists but cannot be parsed.
func NewAgentLoop(source, checkpointPath, bufferDirectory string, bufferBytes int64, batchBytes, batchLines int, exporter LogExporter, metrics *Metrics) (*AgentLoop, error) {
	checkpoint, err := LoadCheckpoint(checkpointPath)
	if err != nil {
		return nil, err
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
// timeouts, read errors, spool encode/enqueue failures — are printed to stderr and
// reported via Metrics so the loop can continue on the next Tick.
func (loop *AgentLoop) Tick(ctx context.Context) error {
	spoolBlocked := false

	// Step 1: drain one entry from the disk spool.
	if loop.BufferDirectory != "" {
		payload, acknowledge, dequeueError := Dequeue(loop.BufferDirectory)
		if dequeueError != nil {
			fmt.Fprintln(os.Stderr, dequeueError)
			loop.Metrics.ObserveSpoolFailure()
			spoolBlocked = true
		} else if payload != nil {
			batch, decodeError := DecodeBatch(payload)
			if decodeError != nil {
				// Corrupt or unrecognisable spool entry: delete it and continue.
				fmt.Fprintln(os.Stderr, decodeError)
				loop.Metrics.ObserveSpoolFailure()
				if ackErr := acknowledge(); ackErr != nil {
					return fmt.Errorf("acknowledge corrupt spool entry: %w", ackErr)
				}
			} else {
				exportCtx, cancel := context.WithTimeout(ctx, exportTimeout)
				exportError := loop.Exporter.Export(exportCtx, batch)
				cancel()
				if exportError != nil {
					fmt.Fprintln(os.Stderr, exportError)
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
			fmt.Fprintln(os.Stderr, readError)
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
			fmt.Fprintln(os.Stderr, exportError)
			loop.Metrics.ObserveExportFailure()
			if loop.BufferDirectory != "" {
				payload, encodeError := EncodeBatch(loop.pending)
				if encodeError != nil {
					fmt.Fprintln(os.Stderr, encodeError)
					loop.Metrics.ObserveSpoolFailure()
				} else if enqueueError := Enqueue(loop.BufferDirectory, loop.BufferBytes, payload); enqueueError != nil {
					fmt.Fprintln(os.Stderr, enqueueError)
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
