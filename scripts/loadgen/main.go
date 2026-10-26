// Command loadgen drives a running TracePilot with a fixed arrival rate of OTLP
// trace exports and management API reads, and reports latency percentiles,
// errors and the server's own queue and connection-pool readings.
//
// Rates are open-loop: requests start on schedule whether or not earlier ones
// finished, so a slow server shows up as latency instead of as a lower rate.
// It uses only the standard library: go run ./scripts/loadgen -help
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type options struct {
	ingestURL, apiURL, apiKey string
	spansPerSecond            int
	spansPerRequest           int
	readsPerSecond            int
	duration, warmup          time.Duration
	maxInFlight               int
	errorFraction             float64
}

// sample is one finished request; latency is measured from the moment it started.
type sample struct {
	class    string
	latency  time.Duration
	status   int // HTTP status; 0 when the request never got a response
	failed   bool
	rejected bool
}

type collector struct {
	mutex   sync.Mutex
	samples []sample
	dropped atomic.Int64 // starts skipped because too many requests were already in flight
}

func (collected *collector) add(item sample) {
	collected.mutex.Lock()
	collected.samples = append(collected.samples, item)
	collected.mutex.Unlock()
}

func main() {
	var settings options
	flag.StringVar(&settings.ingestURL, "ingest-url", "http://127.0.0.1:4318", "OTLP HTTP base URL")
	flag.StringVar(&settings.apiURL, "api-url", "http://127.0.0.1:8080", "management API base URL")
	flag.StringVar(&settings.apiKey, "api-key", os.Getenv("TRACEPILOT_E2E_API_KEY"), "API key with ingest and read scopes")
	flag.IntVar(&settings.spansPerSecond, "spans-per-second", 0, "ingest rate; 0 disables ingest")
	flag.IntVar(&settings.spansPerRequest, "spans-per-request", 20, "spans in each OTLP export")
	flag.IntVar(&settings.readsPerSecond, "reads-per-second", 0, "API read rate; 0 disables reads")
	flag.DurationVar(&settings.duration, "duration", 30*time.Second, "measured run time")
	flag.DurationVar(&settings.warmup, "warmup", 5*time.Second, "time before measuring starts")
	flag.IntVar(&settings.maxInFlight, "max-in-flight", 512, "client-side cap on concurrent requests")
	flag.Float64Var(&settings.errorFraction, "error-fraction", 0.02, "fraction of spans that carry an error status")
	flag.Parse()
	if settings.apiKey == "" || (settings.spansPerSecond <= 0 && settings.readsPerSecond <= 0) || settings.spansPerRequest <= 0 {
		fmt.Fprintln(os.Stderr, "need -api-key and at least one of -spans-per-second or -reads-per-second")
		os.Exit(2)
	}

	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: settings.maxInFlight, MaxConnsPerHost: settings.maxInFlight}}
	collected := &collector{}
	traces := &traceIDs{}
	server := &scraper{client: client, url: settings.apiURL + "/metrics", apiKey: settings.apiKey}
	end := time.Now().Add(settings.warmup + settings.duration)
	measureFrom := time.Now().Add(settings.warmup)

	var running sync.WaitGroup
	if settings.spansPerSecond > 0 {
		running.Add(1)
		go func() {
			defer running.Done()
			drive(settings.requestRate(), end, settings.maxInFlight, collected, func() sample { return exportTraces(client, settings, traces) })
		}()
	}
	if settings.readsPerSecond > 0 {
		running.Add(1)
		go func() {
			defer running.Done()
			drive(float64(settings.readsPerSecond), end, settings.maxInFlight, collected, func() sample { return readAPI(client, settings, traces) })
		}()
	}
	running.Add(1)
	go func() { defer running.Done(); server.watch(measureFrom, end) }()
	running.Wait()

	report(os.Stdout, settings, collected, server, measureFrom)
}

func (settings options) requestRate() float64 {
	return float64(settings.spansPerSecond) / float64(settings.spansPerRequest)
}

// drive starts work at a fixed rate until end.
func drive(perSecond float64, end time.Time, maxInFlight int, collected *collector, work func() sample) {
	interval := time.Duration(float64(time.Second) / perSecond)
	slots := make(chan struct{}, maxInFlight)
	var inFlight sync.WaitGroup
	for next := time.Now(); next.Before(end); next = next.Add(interval) {
		time.Sleep(time.Until(next))
		select {
		case slots <- struct{}{}:
			inFlight.Add(1)
			go func() {
				defer inFlight.Done()
				collected.add(work())
				<-slots
			}()
		default:
			collected.dropped.Add(1)
		}
	}
	inFlight.Wait()
}

// traceIDs remembers recently exported traces so reads can open real ones.
type traceIDs struct {
	mutex sync.Mutex
	items []string
}

func (recent *traceIDs) remember(id string) {
	recent.mutex.Lock()
	defer recent.mutex.Unlock()
	if len(recent.items) < 256 {
		recent.items = append(recent.items, id)
	} else {
		recent.items[rand.IntN(len(recent.items))] = id
	}
}

func (recent *traceIDs) pick() string {
	recent.mutex.Lock()
	defer recent.mutex.Unlock()
	if len(recent.items) == 0 {
		return ""
	}
	return recent.items[rand.IntN(len(recent.items))]
}

func randomHex(bytes int) string {
	buffer := make([]byte, bytes)
	for index := range buffer {
		buffer[index] = byte(rand.IntN(256))
	}
	return hex.EncodeToString(buffer)
}

// exportTraces sends one OTLP/JSON export of spans grouped into small traces.
func exportTraces(client *http.Client, settings options, traces *traceIDs) sample {
	now := time.Now()
	var spans []string
	traceID := ""
	for index := 0; index < settings.spansPerRequest; index++ {
		if index%8 == 0 {
			traceID = randomHex(16)
			traces.remember(traceID)
		}
		status := 1
		if rand.Float64() < settings.errorFraction {
			status = 2
		}
		start := now.Add(-time.Duration(rand.IntN(200)) * time.Millisecond)
		spans = append(spans, fmt.Sprintf(`{"traceId":%q,"spanId":%q,"name":"GET /orders/{id}","kind":2,"startTimeUnixNano":"%d","endTimeUnixNano":"%d","status":{"code":%d,"message":"load"},"attributes":[{"key":"http.route","value":{"stringValue":"/orders/{id}"}}]}`,
			traceID, randomHex(8), start.UnixNano(), start.Add(time.Duration(5+rand.IntN(40))*time.Millisecond).UnixNano(), status))
	}
	body := `{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"loadgen"}}]},"scopeSpans":[{"spans":[` + strings.Join(spans, ",") + `]}]}]}`
	request, _ := http.NewRequest(http.MethodPost, settings.ingestURL+"/v1/traces", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-API-Key", settings.apiKey)
	return perform(client, request, "ingest", true)
}

type endpoint struct {
	path   string
	weight int
}

var readEndpoints = []endpoint{
	{"/api/v1/traces?limit=50", 30}, {"/api/v1/performance", 20}, {"/api/v1/services", 15},
	{"/api/v1/incidents?limit=50", 15}, {"/api/v1/topology", 10}, {"/api/v1/traces/{id}", 10},
}

func readAPI(client *http.Client, settings options, traces *traceIDs) sample {
	total := 0
	for _, item := range readEndpoints {
		total += item.weight
	}
	pick := rand.IntN(total)
	path := readEndpoints[0].path
	for _, item := range readEndpoints {
		if pick < item.weight {
			path = item.path
			break
		}
		pick -= item.weight
	}
	if strings.Contains(path, "{id}") {
		id := traces.pick()
		if id == "" {
			path = "/api/v1/traces?limit=50"
		} else {
			path = strings.Replace(path, "{id}", id, 1)
		}
	}
	request, _ := http.NewRequest(http.MethodGet, settings.apiURL+path, nil)
	request.Header.Set("X-API-Key", settings.apiKey)
	return perform(client, request, "read", false)
}

func perform(client *http.Client, request *http.Request, class string, checkRejected bool) sample {
	started := time.Now()
	response, err := client.Do(request)
	if err != nil {
		return sample{class: class, latency: time.Since(started), failed: true, status: 0}
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	result := sample{class: class, latency: time.Since(started), status: response.StatusCode, failed: response.StatusCode >= 400}
	// A 200 can still carry rejected spans in partialSuccess.
	result.rejected = checkRejected && !result.failed && strings.Contains(string(payload), "rejectedSpans") && !strings.Contains(string(payload), `"rejectedSpans":"0"`)
	return result
}
