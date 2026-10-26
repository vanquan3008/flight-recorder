package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// percentile returns the p-th percentile (0..100) of sorted durations using the
// nearest-rank method.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(p/100*float64(len(sorted))+0.999999999) - 1
	return sorted[max(0, min(rank, len(sorted)-1))]
}

// scraper samples the server's /metrics while the run is measured.
type scraper struct {
	client *http.Client
	url    string
	apiKey string

	mutex       sync.Mutex
	first, last map[string]float64
	peak        map[string]float64
}

// peaked are gauges whose highest reading matters; the rest are counters
// reported as the change over the run.
var peaked = map[string]bool{
	"tracepilot_ingestion_queue_depth": true, "tracepilot_ingestion_inflight_bytes": true,
	`tracepilot_db_pool_connections{state="acquired"}`: true,
}

var counted = []string{
	"tracepilot_db_pool_empty_acquires_total", "tracepilot_db_pool_acquire_wait_seconds_total",
	"tracepilot_ingestion_retries_total", "tracepilot_ingestion_admission_timeouts_total",
	"tracepilot_event_storage_errors_total", "tracepilot_incident_detection_duration_seconds_sum",
	"tracepilot_incident_detection_duration_seconds_count",
}

func (scrape *scraper) watch(from, until time.Time) {
	scrape.first, scrape.last, scrape.peak = map[string]float64{}, map[string]float64{}, map[string]float64{}
	time.Sleep(time.Until(from))
	for tick := time.NewTicker(500 * time.Millisecond); time.Now().Before(until); <-tick.C {
		values := scrape.read()
		scrape.mutex.Lock()
		for name, value := range values {
			if _, seen := scrape.first[name]; !seen {
				scrape.first[name] = value
			}
			scrape.last[name] = value
			if peaked[name] && value > scrape.peak[name] {
				scrape.peak[name] = value
			}
		}
		scrape.mutex.Unlock()
	}
}

func (scrape *scraper) read() map[string]float64 {
	values := map[string]float64{}
	request, _ := http.NewRequest(http.MethodGet, scrape.url, nil)
	request.Header.Set("X-API-Key", scrape.apiKey)
	response, err := scrape.client.Do(request)
	if err != nil || response.StatusCode != http.StatusOK {
		return values
	}
	defer response.Body.Close()
	lines := bufio.NewScanner(response.Body)
	lines.Buffer(make([]byte, 1<<20), 1<<20)
	for lines.Scan() {
		line := lines.Text()
		split := strings.LastIndex(line, " ")
		if split < 0 || strings.HasPrefix(line, "#") {
			continue
		}
		name := line[:split]
		value, parseError := strconv.ParseFloat(line[split+1:], 64)
		if parseError != nil {
			continue
		}
		switch {
		case peaked[name] || contains(counted, name):
			values[name] = value
		case strings.HasPrefix(name, "tracepilot_rejected_records_total{"):
			values[name] = value
		}
	}
	return values
}

func contains(list []string, item string) bool {
	for _, candidate := range list {
		if candidate == item {
			return true
		}
	}
	return false
}

func report(out io.Writer, settings options, collected *collector, scrape *scraper, measureFrom time.Time) {
	byClass := map[string][]time.Duration{}
	failed, rejected := map[string]int{}, 0
	failures := map[string]map[int]int{}
	for _, item := range collected.samples {
		if item.failed {
			if failures[item.class] == nil {
				failures[item.class] = map[int]int{}
			}
			failures[item.class][item.status]++
		}
		byClass[item.class] = append(byClass[item.class], item.latency)
		if item.failed {
			failed[item.class]++
		}
		if item.rejected {
			rejected++
		}
	}
	fmt.Fprintf(out, "duration=%s warmup=%s  client-skipped-starts=%d (in-flight cap %d)\n", settings.duration, settings.warmup, collected.dropped.Load(), settings.maxInFlight)
	for _, class := range []string{"ingest", "read"} {
		latencies := byClass[class]
		if len(latencies) == 0 {
			continue
		}
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		line := fmt.Sprintf("%-6s requests=%d failed=%d (%.2f%%)", class, len(latencies), failed[class], 100*float64(failed[class])/float64(len(latencies)))
		if class == "ingest" {
			line += fmt.Sprintf(" spans/s=%.0f partially-rejected=%d", float64(len(latencies)*settings.spansPerRequest)/(settings.warmup+settings.duration).Seconds(), rejected)
		} else {
			line += fmt.Sprintf(" req/s=%.1f", float64(len(latencies))/(settings.warmup+settings.duration).Seconds())
		}
		if len(failures[class]) > 0 {
			line += fmt.Sprintf(" failures-by-status=%v (0 = no response)", failures[class])
		}
		fmt.Fprintf(out, "%s\n       p50=%s p95=%s p99=%s max=%s\n", line,
			percentile(latencies, 50).Round(time.Millisecond), percentile(latencies, 95).Round(time.Millisecond),
			percentile(latencies, 99).Round(time.Millisecond), latencies[len(latencies)-1].Round(time.Millisecond))
	}
	scrape.mutex.Lock()
	defer scrape.mutex.Unlock()
	for name := range peaked {
		fmt.Fprintf(out, "server peak %s = %.0f\n", name, scrape.peak[name])
	}
	var names []string
	for name := range scrape.last {
		if !peaked[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(out, "server change %s = %.3g\n", name, scrape.last[name]-scrape.first[name])
	}
	_ = measureFrom
}
