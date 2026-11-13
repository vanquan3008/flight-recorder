package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	peakedMetrics = map[string]bool{
		"tracepilot_ingestion_queue_depth": true, "tracepilot_ingestion_inflight_bytes": true,
		"tracepilot_ingestion_inflight_events": true, "tracepilot_db_pool_connections": true,
		"tracepilot_concurrency_gate_active_slots": true,
	}
	countedMetrics = map[string]bool{
		"tracepilot_ingestion_admission_timeouts_total": true, "tracepilot_ingestion_retries_total": true,
		"tracepilot_event_storage_errors_total": true, "tracepilot_rejected_records_total": true,
		"tracepilot_db_pool_empty_acquires_total": true, "tracepilot_db_pool_acquire_wait_seconds_total": true,
		"tracepilot_ingested_records_total": true, "tracepilot_incident_detection_duration_seconds_sum": true,
		"tracepilot_incident_detection_duration_seconds_count": true,
	}
)

// percentile returns the p-th percentile (0..100) of sorted durations by nearest rank.
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
	token  string

	mutex       sync.Mutex
	first, last map[string]float64
	peak        map[string]float64
}

func (scrape *scraper) watch(ctx context.Context, from, until time.Time) {
	scrape.mutex.Lock()
	scrape.first, scrape.last, scrape.peak = map[string]float64{}, map[string]float64{}, map[string]float64{}
	scrape.mutex.Unlock()
	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Until(from)):
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for time.Now().Before(until) {
		values := scrape.read()
		scrape.mutex.Lock()
		for series, value := range values {
			if _, seen := scrape.first[series]; !seen {
				scrape.first[series] = value
			}
			scrape.last[series] = value
			scrape.peak[series] = max(scrape.peak[series], value)
		}
		scrape.mutex.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (scrape *scraper) read() map[string]float64 {
	request, _ := http.NewRequest(http.MethodGet, scrape.url, nil)
	if scrape.token != "" {
		request.Header.Set("Authorization", "Bearer "+scrape.token)
	}
	response, err := scrape.client.Do(request)
	if err != nil {
		return nil
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil
	}
	return parseMetrics(response.Body)
}

func parseMetrics(body io.Reader) map[string]float64 {
	values := map[string]float64{}
	lines := bufio.NewScanner(body)
	lines.Buffer(make([]byte, 1<<20), 1<<20)
	for lines.Scan() {
		line := lines.Text()
		split := strings.LastIndex(line, " ")
		if split < 0 || strings.HasPrefix(line, "#") {
			continue
		}
		series := line[:split]
		name, _, _ := strings.Cut(series, "{")
		if !peakedMetrics[name] && !countedMetrics[name] {
			continue
		}
		if value, err := strconv.ParseFloat(line[split+1:], 64); err == nil {
			values[series] = value
		}
	}
	return values
}

func report(out io.Writer, settings options, collected *collector, scrape *scraper, from, until time.Time) {
	window := until.Sub(from).Seconds()
	byClass := map[string][]sample{}
	collected.mutex.Lock()
	for _, item := range collected.samples {
		if !item.at.Before(from) && item.at.Before(until) {
			byClass[item.class] = append(byClass[item.class], item)
		}
	}
	collected.mutex.Unlock()
	fmt.Fprintf(out, "measured=%s warmup=%s client-skipped-starts=%d (in-flight cap %d per class)\n",
		settings.duration, settings.warmup, collected.skipped.Load(), settings.maxInFlight)
	for _, class := range []string{"traces", "logs", "reads"} {
		items := byClass[class]
		if len(items) == 0 {
			continue
		}
		latencies := make([]time.Duration, 0, len(items))
		failures := map[int]int{}
		var failed, records int
		var rejected int64
		for _, item := range items {
			latencies = append(latencies, item.latency)
			rejected += item.rejected
			if item.failed {
				failed++
				failures[item.status]++
			} else {
				records += item.records
			}
		}
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		line := fmt.Sprintf("%-6s requests=%d (%.1f/s) failed=%d (%.2f%%)", class, len(items), float64(len(items))/window, failed, 100*float64(failed)/float64(len(items)))
		if class != "reads" {
			line += fmt.Sprintf(" accepted-records=%.0f/s partially-rejected=%d", float64(records)/window, rejected)
		}
		if len(failures) > 0 {
			line += fmt.Sprintf(" failures-by-status=%v (0 = no response)", failures)
		}
		fmt.Fprintf(out, "%s\n       p50=%s p95=%s p99=%s max=%s\n", line,
			percentile(latencies, 50).Round(time.Millisecond), percentile(latencies, 95).Round(time.Millisecond),
			percentile(latencies, 99).Round(time.Millisecond), latencies[len(latencies)-1].Round(time.Millisecond))
	}
	reportEndpoints(out, byClass["reads"])
	scrape.mutex.Lock()
	defer scrape.mutex.Unlock()
	series := make([]string, 0, len(scrape.last))
	for name := range scrape.last {
		series = append(series, name)
	}
	sort.Strings(series)
	for _, name := range series {
		metric, _, _ := strings.Cut(name, "{")
		if peakedMetrics[metric] {
			fmt.Fprintf(out, "server peak   %s = %.0f\n", name, scrape.peak[name])
		} else if change := scrape.last[name] - scrape.first[name]; change != 0 {
			fmt.Fprintf(out, "server change %s = %.4g\n", name, change)
		}
	}
}

func reportEndpoints(out io.Writer, reads []sample) {
	byEndpoint := map[string][]sample{}
	for _, item := range reads {
		byEndpoint[item.endpoint] = append(byEndpoint[item.endpoint], item)
	}
	endpoints := make([]string, 0, len(byEndpoint))
	for endpoint := range byEndpoint {
		endpoints = append(endpoints, endpoint)
	}
	sort.Strings(endpoints)
	for _, endpoint := range endpoints {
		items := byEndpoint[endpoint]
		latencies := make([]time.Duration, 0, len(items))
		failed := 0
		for _, item := range items {
			latencies = append(latencies, item.latency)
			if item.failed {
				failed++
			}
		}
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		fmt.Fprintf(out, "  read %-28s n=%d failed=%d p50=%s p95=%s\n", endpoint, len(items), failed,
			percentile(latencies, 50).Round(time.Millisecond), percentile(latencies, 95).Round(time.Millisecond))
	}
}
