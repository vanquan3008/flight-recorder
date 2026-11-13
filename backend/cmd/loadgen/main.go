// Command loadgen drives a running TracePilot with a fixed arrival rate of OTLP
// trace and log exports plus console API reads, then reports latency
// percentiles, failures and the server's own queue and pool readings.
//
// Rates are open-loop: requests start on schedule whether or not earlier ones
// finished, so a slow server shows up as latency instead of as a lower rate.
// It creates its own project, environment and ingest token on every run.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"
)

type options struct {
	apiURL, ingestURL     string
	email, password       string
	metricsToken          string
	spansPerSecond        int
	spansPerRequest       int
	logsPerSecond         int
	logsPerRequest        int
	readsPerSecond        int
	duration, warmup      time.Duration
	maxInFlight           int
	errorFraction         float64
	requestTimeout        time.Duration
	environment, runLabel string
}

type sample struct {
	class    string
	endpoint string
	at       time.Time
	latency  time.Duration
	status   int
	failed   bool
	rejected int64
	records  int
}

type collector struct {
	mutex   sync.Mutex
	samples []sample
	skipped atomic.Int64
}

func (collected *collector) add(item sample) {
	collected.mutex.Lock()
	collected.samples = append(collected.samples, item)
	collected.mutex.Unlock()
}

func parseOptions(arguments []string) (options, error) {
	var settings options
	flags := flag.NewFlagSet("loadgen", flag.ContinueOnError)
	flags.StringVar(&settings.apiURL, "api-url", "http://127.0.0.1:8080", "management API base URL")
	flags.StringVar(&settings.ingestURL, "ingest-url", "http://127.0.0.1:4318", "OTLP HTTP base URL")
	flags.StringVar(&settings.email, "email", envOr("TRACEPILOT_LOAD_EMAIL", "admin@tracepilot.local"), "account that may create projects")
	flags.StringVar(&settings.password, "password", envOr("TRACEPILOT_LOAD_PASSWORD", "admin-password-123"), "password for -email")
	flags.StringVar(&settings.metricsToken, "metrics-token", os.Getenv("TRACEPILOT_HTTP_METRICS_TOKEN"), "scrape token when /metrics requires authentication")
	flags.IntVar(&settings.spansPerSecond, "spans-per-second", 0, "span ingest rate; 0 disables traces")
	flags.IntVar(&settings.spansPerRequest, "spans-per-request", 64, "spans in each OTLP trace export")
	flags.IntVar(&settings.logsPerSecond, "logs-per-second", 0, "log ingest rate; 0 disables logs")
	flags.IntVar(&settings.logsPerRequest, "logs-per-request", 64, "records in each OTLP log export")
	flags.IntVar(&settings.readsPerSecond, "reads-per-second", 0, "console API read rate; 0 disables reads")
	flags.DurationVar(&settings.duration, "duration", time.Minute, "measured run time")
	flags.DurationVar(&settings.warmup, "warmup", 10*time.Second, "load before measuring starts")
	flags.IntVar(&settings.maxInFlight, "max-in-flight", 256, "client-side cap on concurrent requests per class")
	flags.Float64Var(&settings.errorFraction, "error-fraction", 0.02, "fraction of spans and logs that report an error")
	flags.DurationVar(&settings.requestTimeout, "request-timeout", 30*time.Second, "client timeout for each request")
	flags.StringVar(&settings.environment, "environment", "load", "environment created for the run")
	flags.StringVar(&settings.runLabel, "run", fmt.Sprintf("%d", time.Now().Unix()), "suffix for the project slug")
	if err := flags.Parse(arguments); err != nil {
		return settings, err
	}
	switch {
	case settings.spansPerSecond <= 0 && settings.logsPerSecond <= 0 && settings.readsPerSecond <= 0:
		return settings, fmt.Errorf("set at least one of -spans-per-second, -logs-per-second or -reads-per-second")
	case settings.spansPerRequest <= 0 || settings.logsPerRequest <= 0 || settings.maxInFlight <= 0:
		return settings, fmt.Errorf("-spans-per-request, -logs-per-request and -max-in-flight must be positive")
	case settings.duration <= 0 || settings.warmup < 0:
		return settings, fmt.Errorf("-duration must be positive and -warmup not negative")
	}
	return settings, nil
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func main() {
	settings, err := parseOptions(os.Args[1:])
	if err != nil {
		slog.Error("invalid arguments", "error", err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, settings); err != nil {
		slog.Error("load run failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, settings options) error {
	transport := &http.Transport{MaxIdleConnsPerHost: settings.maxInFlight, MaxConnsPerHost: 3 * settings.maxInFlight}
	client := &http.Client{Timeout: settings.requestTimeout, Transport: transport}
	session, err := newSession(ctx, client, settings)
	if err != nil {
		return err
	}
	fmt.Printf("project=%s environment=%s\n", session.projectID, settings.environment)

	collected := &collector{}
	traces := &traceIDs{}
	scrape := &scraper{client: client, url: settings.apiURL + "/metrics", token: settings.metricsToken}
	start := time.Now()
	measureFrom, end := start.Add(settings.warmup), start.Add(settings.warmup+settings.duration)

	var running sync.WaitGroup
	launch := func(perSecond float64, work func() sample) {
		running.Add(1)
		go func() {
			defer running.Done()
			drive(ctx, perSecond, end, settings.maxInFlight, collected, work)
		}()
	}
	if settings.spansPerSecond > 0 {
		launch(float64(settings.spansPerSecond)/float64(settings.spansPerRequest), func() sample { return exportTraces(client, settings, session, traces) })
	}
	if settings.logsPerSecond > 0 {
		launch(float64(settings.logsPerSecond)/float64(settings.logsPerRequest), func() sample { return exportLogs(client, settings, session, traces) })
	}
	if settings.readsPerSecond > 0 {
		launch(float64(settings.readsPerSecond), func() sample { return readAPI(ctx, client, settings, session, traces) })
	}
	running.Add(1)
	go func() { defer running.Done(); scrape.watch(ctx, measureFrom, end) }()
	running.Wait()

	report(os.Stdout, settings, collected, scrape, measureFrom, end)
	return ctx.Err()
}

func drive(ctx context.Context, perSecond float64, end time.Time, maxInFlight int, collected *collector, work func() sample) {
	interval := time.Duration(float64(time.Second) / perSecond)
	slots := make(chan struct{}, maxInFlight)
	var inFlight sync.WaitGroup
	timer := time.NewTimer(0)
	defer timer.Stop()
	for next := time.Now(); next.Before(end); next = next.Add(interval) {
		timer.Reset(time.Until(next))
		select {
		case <-ctx.Done():
			inFlight.Wait()
			return
		case <-timer.C:
		}
		select {
		case slots <- struct{}{}:
			inFlight.Add(1)
			go func() {
				defer inFlight.Done()
				collected.add(work())
				<-slots
			}()
		default:
			collected.skipped.Add(1)
		}
	}
	inFlight.Wait()
}
