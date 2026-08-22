package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/flight-observability/flight/internal/agent"
)

func main() {
	os.Exit(run())
}

func run() int {
	source := flag.String("source", "", "log file to tail")
	checkpointPath := flag.String("checkpoint", "", "checkpoint file")
	endpoint := flag.String("endpoint", "localhost:4317", "OTLP gRPC endpoint")
	apiKey := flag.String("api-key", "", "OTLP API key")
	service := flag.String("service", "file-agent", "service.name resource attribute")
	bufferDirectory := flag.String("disk-buffer", "", "optional bounded outage buffer directory")
	bufferBytes := flag.Int64("disk-buffer-bytes", 64<<20, "maximum disk buffer size")
	batchBytes := flag.Int("batch-bytes", 3<<20, "maximum source bytes per line and OTLP export")
	batchLines := flag.Int("batch-lines", 1000, "maximum source lines per OTLP export")
	secure := flag.Bool("tls", false, "use TLS for the OTLP gRPC connection")
	tlsServerName := flag.String("tls-server-name", "", "TLS server name override")
	metricsAddress := flag.String("metrics-address", "", "optional Prometheus metrics listen address")
	flag.Parse()
	if *source == "" || *checkpointPath == "" {
		fmt.Fprintln(os.Stderr, "source and checkpoint are required")
		return 2
	}
	if *batchBytes < 64<<10 || *batchLines <= 0 || *bufferBytes < 0 {
		fmt.Fprintln(os.Stderr, "batch limits must be positive and batch-bytes must be at least 65536")
		return 2
	}
	checkpoint, checkpointError := agent.LoadCheckpoint(*checkpointPath)
	if checkpointError != nil {
		fmt.Fprintln(os.Stderr, checkpointError)
		return 1
	}
	logExporter, exporterError := agent.NewExporter(*endpoint, *apiKey, *service, *secure, *tlsServerName)
	if exporterError != nil {
		fmt.Fprintln(os.Stderr, exporterError)
		return 1
	}
	defer logExporter.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	metrics := agent.NewMetrics()
	if *metricsAddress != "" {
		listener, listenError := net.Listen("tcp", *metricsAddress)
		if listenError != nil {
			fmt.Fprintln(os.Stderr, listenError)
			return 1
		}
		metricsServer := &http.Server{Handler: metrics.Handler(), ReadHeaderTimeout: 5 * time.Second}
		go func() {
			if serveError := metricsServer.Serve(listener); serveError != nil && serveError != http.ErrServerClosed {
				slog.Error("agent metrics server stopped", "error", serveError)
			}
		}()
		defer func() {
			shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = metricsServer.Shutdown(shutdownContext)
		}()
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var pending agent.Batch
	var pendingCheckpoint agent.Checkpoint
	for {
		spoolBlocked := false
		if *bufferDirectory != "" {
			payload, acknowledge, dequeueError := agent.Dequeue(*bufferDirectory)
			if dequeueError != nil {
				fmt.Fprintln(os.Stderr, dequeueError)
				spoolBlocked = true
			}
			if payload != nil {
				batch, decodeError := agent.DecodeBatch(payload)
				if decodeError == nil {
					exportContext, cancel := context.WithTimeout(ctx, 5*time.Second)
					exportError := logExporter.Export(exportContext, batch)
					cancel()
					if exportError == nil {
						metrics.ObserveExport(len(batch))
						if acknowledgeError := acknowledge(); acknowledgeError != nil {
							fmt.Fprintln(os.Stderr, acknowledgeError)
							metrics.ObserveSpoolFailure()
							return 1
						}
					} else {
						fmt.Fprintln(os.Stderr, exportError)
						metrics.ObserveExportFailure()
						spoolBlocked = true
					}
				} else {
					fmt.Fprintln(os.Stderr, decodeError)
					metrics.ObserveSpoolFailure()
					if acknowledgeError := acknowledge(); acknowledgeError != nil {
						fmt.Fprintln(os.Stderr, acknowledgeError)
						return 1
					}
				}
			}
		}
		if !spoolBlocked && len(pending) == 0 {
			lines, next, readError := agent.ReadLines(*source, checkpoint, *batchBytes, *batchBytes, *batchLines)
			if readError == nil && len(lines) > 0 {
				metrics.ObserveRead(len(lines))
				pending = agent.NewBatch(lines)
				pendingCheckpoint = next
			} else if readError != nil {
				fmt.Fprintln(os.Stderr, readError)
			}
		}
		if !spoolBlocked && len(pending) > 0 {
			exportContext, cancel := context.WithTimeout(ctx, 5*time.Second)
			exportError := logExporter.Export(exportContext, pending)
			cancel()
			if exportError == nil {
				metrics.ObserveExport(len(pending))
				checkpoint = pendingCheckpoint
				if saveError := agent.SaveCheckpoint(*checkpointPath, checkpoint); saveError != nil {
					fmt.Fprintln(os.Stderr, saveError)
					return 1
				}
				pending = nil
			} else {
				fmt.Fprintln(os.Stderr, exportError)
				metrics.ObserveExportFailure()
				if *bufferDirectory != "" {
					payload, encodeError := agent.EncodeBatch(pending)
					if encodeError != nil {
						fmt.Fprintln(os.Stderr, encodeError)
						metrics.ObserveSpoolFailure()
					} else if enqueueError := agent.Enqueue(*bufferDirectory, *bufferBytes, payload); enqueueError != nil {
						fmt.Fprintln(os.Stderr, enqueueError)
						metrics.ObserveSpoolFailure()
					} else {
						checkpoint = pendingCheckpoint
						if saveError := agent.SaveCheckpoint(*checkpointPath, checkpoint); saveError != nil {
							fmt.Fprintln(os.Stderr, saveError)
							return 1
						}
						pending = nil
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return 0
		case <-ticker.C:
		}
	}
}
