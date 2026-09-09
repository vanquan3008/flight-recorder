package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/vanquan3008/tracepilot/internal/agent"
)

func main() {
	os.Exit(run())
}

func run() int {
	source := flag.String("source", "", "log file to tail")
	checkpointPath := flag.String("checkpoint", "", "checkpoint file")
	endpoint := flag.String("endpoint", "localhost:4317", "OTLP gRPC endpoint")
	apiKey := flag.String("api-key", "", "OTLP API key (development only; prefer TRACEPILOT_AGENT_API_KEY or -api-key-file)")
	apiKeyFile := flag.String("api-key-file", "", "read the OTLP API key from a secret file")
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
	resolvedSource, sourceError := filepath.Abs(*source)
	if sourceError != nil {
		fmt.Fprintln(os.Stderr, "resolve source path:", sourceError)
		return 2
	}
	hostName, hostError := os.Hostname()
	if hostError != nil || hostName == "" {
		fmt.Fprintln(os.Stderr, "read machine hostname:", hostError)
		return 1
	}
	if *batchBytes < 64<<10 || *batchLines <= 0 || *bufferBytes < 0 {
		fmt.Fprintln(os.Stderr, "batch limits must be positive and batch-bytes must be at least 65536")
		return 2
	}
	resolvedAPIKey, apiKeyError := resolveAPIKey(*apiKey, *apiKeyFile, os.Getenv("TRACEPILOT_AGENT_API_KEY"))
	if apiKeyError != nil {
		fmt.Fprintln(os.Stderr, apiKeyError)
		return 2
	}
	if *apiKey != "" {
		fmt.Fprintln(os.Stderr, "warning: -api-key exposes the secret in process arguments; use TRACEPILOT_AGENT_API_KEY or -api-key-file")
	}
	logExporter, exporterError := agent.NewExporter(*endpoint, resolvedAPIKey, *service, hostName, resolvedSource, *secure, *tlsServerName)
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
	loop, loopError := agent.NewAgentLoop(
		resolvedSource, *checkpointPath, *bufferDirectory, *bufferBytes,
		*batchBytes, *batchLines, logExporter, metrics,
	)
	if loopError != nil {
		fmt.Fprintln(os.Stderr, loopError)
		return 1
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if tickError := loop.Tick(ctx); tickError != nil {
			fmt.Fprintln(os.Stderr, tickError)
			return 1
		}
		select {
		case <-ctx.Done():
			return 0
		case <-ticker.C:
		}
	}
}

func resolveAPIKey(explicit, file, environment string) (string, error) {
	if explicit != "" && file != "" {
		return "", errors.New("use only one of -api-key and -api-key-file")
	}
	if file != "" {
		contents, readError := os.ReadFile(file)
		if readError != nil {
			return "", fmt.Errorf("read API key file: %w", readError)
		}
		key := strings.TrimSpace(string(contents))
		if key == "" {
			return "", errors.New("API key file is empty")
		}
		return key, nil
	}
	if explicit != "" {
		return explicit, nil
	}
	return strings.TrimSpace(environment), nil
}
