// Command loadgen drives a running TracePilot with a fixed arrival rate of OTLP
// trace, log, and metric exports plus console API reads, then reports latency
// percentiles, failures and the server's own queue and pool readings.
//
// Rates are open-loop: requests start on schedule whether or not earlier ones
// finished, so a slow server shows up as latency instead of as a lower rate.
// It creates its own projects, environments and ingest tokens on every run.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"math/rand/v2"
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
	pointsPerSecond       int
	pointsPerRequest      int
	readsPerSecond        int
	readWindow            time.Duration
	projects              int
	prefill, history      time.Duration
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
	flags.StringVar(&settings.email, "email", os.Getenv("TRACEPILOT_LOAD_EMAIL"), "account that may create projects")
	flags.StringVar(&settings.password, "password", os.Getenv("TRACEPILOT_LOAD_PASSWORD"), "password for -email")
	flags.StringVar(&settings.metricsToken, "metrics-token", os.Getenv("TRACEPILOT_HTTP_METRICS_TOKEN"), "scrape token when /metrics requires authentication")
	flags.IntVar(&settings.spansPerSecond, "spans-per-second", 0, "span ingest rate; 0 disables traces")
	flags.IntVar(&settings.spansPerRequest, "spans-per-request", 64, "spans in each OTLP trace export")
	flags.IntVar(&settings.logsPerSecond, "logs-per-second", 0, "log ingest rate; 0 disables logs")
	flags.IntVar(&settings.logsPerRequest, "logs-per-request", 64, "records in each OTLP log export")
	flags.IntVar(&settings.pointsPerSecond, "metric-points-per-second", 0, "metric point ingest rate; 0 disables metrics")
	flags.IntVar(&settings.pointsPerRequest, "metric-points-per-request", 64, "points in each OTLP metric export")
	flags.IntVar(&settings.readsPerSecond, "reads-per-second", 0, "console API read rate; 0 disables reads")
	flags.DurationVar(&settings.readWindow, "read-window", 0, "time range for console list and analytics reads; 0 uses API defaults")
	flags.IntVar(&settings.projects, "projects", 1, "fresh projects sharing the offered rates equally")
	flags.DurationVar(&settings.prefill, "prefill", 0, "ingest-only time before warmup and measurement")
	flags.DurationVar(&settings.history, "history", 0, "spread prefill timestamps over this much history")
	flags.DurationVar(&settings.duration, "duration", time.Minute, "measured run time")
	flags.DurationVar(&settings.warmup, "warmup", 10*time.Second, "load before measuring starts")
	flags.IntVar(&settings.maxInFlight, "max-in-flight", 256, "client-side cap on concurrent requests per class per project")
	flags.Float64Var(&settings.errorFraction, "error-fraction", 0.02, "fraction of spans and logs that report an error")
	flags.DurationVar(&settings.requestTimeout, "request-timeout", 30*time.Second, "client timeout for each request")
	flags.StringVar(&settings.environment, "environment", "load", "environment created for the run")
	flags.StringVar(&settings.runLabel, "run", fmt.Sprintf("%d", time.Now().Unix()), "suffix for the project slug")
	if err := flags.Parse(arguments); err != nil {
		return settings, err
	}
	switch {
	case settings.email == "" || settings.password == "":
		return settings, fmt.Errorf("email and password required: set -email and -password or TRACEPILOT_LOAD_EMAIL / TRACEPILOT_LOAD_PASSWORD")
	case settings.spansPerSecond < 0 || settings.logsPerSecond < 0 || settings.pointsPerSecond < 0 || settings.readsPerSecond < 0:
		return settings, fmt.Errorf("workload rates must not be negative")
	case settings.spansPerSecond == 0 && settings.logsPerSecond == 0 && settings.pointsPerSecond == 0 && settings.readsPerSecond == 0:
		return settings, fmt.Errorf("set a positive trace, log, metric point, or console read rate")
	case settings.spansPerRequest <= 0 || settings.logsPerRequest <= 0 || settings.pointsPerRequest <= 0 || settings.projects <= 0 || settings.maxInFlight <= 0:
		return settings, fmt.Errorf("batch sizes, -projects and -max-in-flight must be positive")
	case settings.duration <= 0 || settings.warmup < 0 || settings.readWindow < 0:
		return settings, fmt.Errorf("-duration must be positive and -warmup and -read-window must not be negative")
	case settings.prefill < 0 || settings.history < 0 || (settings.history > 0 && settings.prefill == 0):
		return settings, fmt.Errorf("-prefill and -history must not be negative, and -history requires -prefill")
	case settings.prefill > 0 && settings.spansPerSecond == 0 && settings.logsPerSecond == 0 && settings.pointsPerSecond == 0:
		return settings, fmt.Errorf("-prefill requires a trace, log, or metric point rate")
	}
	return settings, nil
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

type projectLoad struct {
	session *session
	traces  traceIDs
}

func run(ctx context.Context, settings options) error {
	transport := &http.Transport{MaxIdleConnsPerHost: settings.maxInFlight}
	client := &http.Client{Timeout: settings.requestTimeout, Transport: transport}
	defer client.CloseIdleConnections()
	projects := make([]projectLoad, 0, settings.projects)
	for index := range settings.projects {
		projectSettings := settings
		if settings.projects > 1 {
			projectSettings.runLabel = fmt.Sprintf("%s-%d", settings.runLabel, index+1)
		}
		session, err := newSession(ctx, client, projectSettings)
		if err != nil {
			return err
		}
		fmt.Printf("project=%s environment=%s\n", session.projectID, settings.environment)
		projects = append(projects, projectLoad{session: session})
	}
	if settings.prefill > 0 {
		prefill := settings
		prefill.readsPerSecond, prefill.duration, prefill.warmup = 0, settings.prefill, 0
		from := time.Now()
		until := from.Add(prefill.duration)
		collected := &collector{}
		runWorkloads(ctx, client, prefill, projects, collected, until)
		fmt.Printf("prefill history=%s\n", settings.history)
		report(os.Stdout, prefill, collected, &scraper{}, from, until)
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	settings.history = 0
	scrape := &scraper{client: client, url: settings.apiURL + "/metrics", token: settings.metricsToken}
	start := time.Now()
	measureFrom, end := start.Add(settings.warmup), start.Add(settings.warmup+settings.duration)
	collected := &collector{}
	scraped := make(chan struct{})
	go func() { defer close(scraped); scrape.watch(ctx, measureFrom, end) }()
	runWorkloads(ctx, client, settings, projects, collected, end)
	<-scraped
	report(os.Stdout, settings, collected, scrape, measureFrom, end)
	return ctx.Err()
}

func runWorkloads(ctx context.Context, client *http.Client, settings options, projects []projectLoad, collected *collector, end time.Time) {
	var running sync.WaitGroup
	launch := func(recordsPerSecond, batchSize int, work func() sample) {
		if recordsPerSecond == 0 {
			return
		}
		perSecond := float64(recordsPerSecond) / float64(batchSize) / float64(len(projects))
		running.Add(1)
		go func() {
			defer running.Done()
			drive(ctx, perSecond, end, settings.maxInFlight, collected, work)
		}()
	}
	for index := range projects {
		project := &projects[index]
		launch(settings.spansPerSecond, settings.spansPerRequest, func() sample {
			return exportTraces(client, settings, project.session, &project.traces, workloadTime(settings.history))
		})
		launch(settings.logsPerSecond, settings.logsPerRequest, func() sample {
			return exportLogs(client, settings, project.session, &project.traces, workloadTime(settings.history))
		})
		launch(settings.pointsPerSecond, settings.pointsPerRequest, func() sample {
			return exportMetrics(client, settings, project.session, workloadTime(settings.history))
		})
		launch(settings.readsPerSecond, 1, func() sample { return readAPI(ctx, client, settings, project.session, &project.traces) })
	}
	running.Wait()
}

func workloadTime(history time.Duration) time.Time {
	now := time.Now()
	if history > 0 {
		return now.Add(-rand.N(history))
	}
	return now
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
