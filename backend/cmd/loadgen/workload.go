package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const spansPerTrace = 8

var (
	spanShape = []struct{ service, name string }{
		{"frontend", "GET /checkout"}, {"checkout", "POST /orders"}, {"checkout", "SELECT orders"},
		{"payments", "POST /charges"}, {"payments", "SELECT accounts"}, {"inventory", "GET /stock/{sku}"},
		{"inventory", "SELECT stock"}, {"checkout", "publish order.created"},
	}
	errorMessages = []string{"upstream timeout", "connection refused", "card declined", "deadlock detected"}
)

// traceIDs remembers recent traces so logs correlate with them and reads open real ones.
type traceIDs struct {
	mutex sync.Mutex
	items []string
}

func (recent *traceIDs) remember(id string) {
	recent.mutex.Lock()
	defer recent.mutex.Unlock()
	if len(recent.items) < 512 {
		recent.items = append(recent.items, id)
		return
	}
	recent.items[rand.IntN(len(recent.items))] = id
}

func (recent *traceIDs) pick() string {
	recent.mutex.Lock()
	defer recent.mutex.Unlock()
	if len(recent.items) == 0 {
		return ""
	}
	return recent.items[rand.IntN(len(recent.items))]
}

func randomHex(size int) string {
	buffer := make([]byte, size)
	for index := range buffer {
		buffer[index] = byte(rand.IntN(256))
	}
	return hex.EncodeToString(buffer)
}

func stringAttribute(key, value string) map[string]any {
	return map[string]any{"key": key, "value": map[string]any{"stringValue": value}}
}

func traceExport(spanCount int, errorFraction float64, now time.Time) (map[string]any, []string) {
	byService := map[string][]any{}
	var traceIDs []string
	var traceID, rootID string
	var start time.Time
	for index := range spanCount {
		position := index % spansPerTrace
		if position == 0 {
			traceID, rootID = randomHex(16), randomHex(8)
			start = now.Add(-time.Duration(rand.IntN(2000)) * time.Millisecond)
			traceIDs = append(traceIDs, traceID)
		}
		spanID, parent := rootID, ""
		if position > 0 {
			spanID, parent = randomHex(8), rootID
		}
		status := map[string]any{"code": 1}
		if rand.Float64() < errorFraction {
			status = map[string]any{"code": 2, "message": errorMessages[rand.IntN(len(errorMessages))]}
		}
		shape := spanShape[position%len(spanShape)]
		begin := start.Add(time.Duration(position) * time.Millisecond)
		span := map[string]any{
			"traceId": traceID, "spanId": spanID, "parentSpanId": parent, "name": shape.name, "kind": 2,
			"startTimeUnixNano": strconv.FormatInt(begin.UnixNano(), 10),
			"endTimeUnixNano":   strconv.FormatInt(begin.Add(time.Duration(2+rand.IntN(80))*time.Millisecond).UnixNano(), 10),
			"status":            status,
			"attributes":        []any{stringAttribute("http.route", shape.name), stringAttribute("load.run", "loadgen")},
		}
		byService[shape.service] = append(byService[shape.service], span)
	}
	var resources []any
	for service, spans := range byService {
		resources = append(resources, map[string]any{
			"resource":   map[string]any{"attributes": []any{stringAttribute("service.name", service)}},
			"scopeSpans": []any{map[string]any{"scope": map[string]any{"name": "loadgen"}, "spans": spans}},
		})
	}
	return map[string]any{"resourceSpans": resources}, traceIDs
}

func logExport(recordCount int, errorFraction float64, now time.Time, traces *traceIDs) map[string]any {
	records := make([]any, 0, recordCount)
	for range recordCount {
		severity, text, body := 9, "INFO", "order accepted"
		if rand.Float64() < errorFraction {
			severity, text, body = 17, "ERROR", errorMessages[rand.IntN(len(errorMessages))]
		}
		record := map[string]any{
			"timeUnixNano":   strconv.FormatInt(now.Add(-time.Duration(rand.IntN(2000))*time.Millisecond).UnixNano(), 10),
			"severityNumber": severity, "severityText": text,
			"body":       map[string]any{"stringValue": body + " id=" + randomHex(4)},
			"attributes": []any{stringAttribute("load.run", "loadgen")},
		}
		if traceID := traces.pick(); traceID != "" {
			record["traceId"], record["spanId"] = traceID, randomHex(8)
		}
		records = append(records, record)
	}
	return map[string]any{"resourceLogs": []any{map[string]any{
		"resource":  map[string]any{"attributes": []any{stringAttribute("service.name", "checkout")}},
		"scopeLogs": []any{map[string]any{"scope": map[string]any{"name": "loadgen"}, "logRecords": records}},
	}}}
}

func metricExport(pointCount int, now time.Time) map[string]any {
	points := make([]any, 0, pointCount)
	for index := range pointCount {
		points = append(points, map[string]any{
			"timeUnixNano": strconv.FormatInt(now.Add(-time.Duration(index)).UnixNano(), 10),
			"asDouble":     rand.Float64() * 100,
		})
	}
	return map[string]any{"resourceMetrics": []any{map[string]any{
		"resource": map[string]any{"attributes": []any{stringAttribute("service.name", "checkout")}},
		"scopeMetrics": []any{map[string]any{
			"scope":   map[string]any{"name": "loadgen"},
			"metrics": []any{map[string]any{"name": "loadgen.queue.depth", "unit": "1", "gauge": map[string]any{"dataPoints": points}}},
		}},
	}}}
}

func exportTraces(client *http.Client, settings options, current *session, traces *traceIDs, now time.Time) sample {
	body, exported := traceExport(settings.spansPerRequest, settings.errorFraction, now)
	result := exportOTLP(client, settings, current, "/v1/traces", "traces", body, settings.spansPerRequest)
	if !result.failed && result.rejected == 0 {
		for _, traceID := range exported {
			traces.remember(traceID)
		}
	}
	return result
}

func exportLogs(client *http.Client, settings options, current *session, traces *traceIDs, now time.Time) sample {
	body := logExport(settings.logsPerRequest, settings.errorFraction, now, traces)
	return exportOTLP(client, settings, current, "/v1/logs", "logs", body, settings.logsPerRequest)
}

func exportMetrics(client *http.Client, settings options, current *session, now time.Time) sample {
	body := metricExport(settings.pointsPerRequest, now)
	return exportOTLP(client, settings, current, "/v1/metrics", "metrics", body, settings.pointsPerRequest)
}

func exportOTLP(client *http.Client, settings options, current *session, path, class string, body any, records int) sample {
	payload, _ := json.Marshal(body)
	request, _ := http.NewRequest(http.MethodPost, settings.ingestURL+path, strings.NewReader(string(payload)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+current.ingestToken)
	request.Header.Set("X-Environment", settings.environment)
	result := perform(client, request, class)
	result.records = records
	return result
}

var readEndpoints = []struct {
	path   string
	weight int
}{
	{"/api/v1/traces?limit=50", 25}, {"/api/v1/logs?limit=50", 20}, {"/api/v1/performance", 15},
	{"/api/v1/services", 10}, {"/api/v1/incidents?limit=25", 10}, {"/api/v1/topology", 10}, {"/api/v1/traces/{id}", 10},
}

// pickReadPath returns the request path and the endpoint template it reports under.
func pickReadPath(roll int, traceID string) (string, string) {
	for _, endpoint := range readEndpoints {
		if roll < endpoint.weight {
			if !strings.Contains(endpoint.path, "{id}") {
				return endpoint.path, endpoint.path
			}
			if traceID == "" {
				return readEndpoints[0].path, readEndpoints[0].path
			}
			return strings.Replace(endpoint.path, "{id}", traceID, 1), endpoint.path
		}
		roll -= endpoint.weight
	}
	return readEndpoints[0].path, readEndpoints[0].path
}

func readAPI(ctx context.Context, client *http.Client, settings options, current *session, traces *traceIDs) sample {
	total := 0
	for _, endpoint := range readEndpoints {
		total += endpoint.weight
	}
	path, endpoint := pickReadPath(rand.IntN(total), traces.pick())
	if settings.readWindow > 0 && !strings.HasSuffix(endpoint, "/{id}") {
		to := time.Now().UTC()
		separator := "?"
		if strings.Contains(path, "?") {
			separator = "&"
		}
		path += separator + url.Values{"from": {to.Add(-settings.readWindow).Format(time.RFC3339Nano)}, "to": {to.Format(time.RFC3339Nano)}}.Encode()
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, settings.apiURL+path, nil)
	current.scopeHeaders(ctx, request.Header)
	result := perform(client, request, "reads")
	result.endpoint = endpoint
	return result
}

func perform(client *http.Client, request *http.Request, class string) sample {
	started := time.Now()
	response, err := client.Do(request)
	if err != nil {
		return sample{class: class, at: started, latency: time.Since(started), failed: true}
	}
	defer response.Body.Close()
	var payload []byte
	if class == "reads" {
		_, err = io.Copy(io.Discard, response.Body)
	} else {
		payload, err = io.ReadAll(io.LimitReader(response.Body, 4<<20))
	}
	result := sample{class: class, at: started, latency: time.Since(started), status: response.StatusCode, failed: response.StatusCode >= 300}
	if err != nil {
		result.failed = true
	}
	if !result.failed && class != "reads" {
		result.rejected = rejectedRecords(payload)
	}
	return result
}

// rejectedRecords reads OTLP partial success; protojson writes int64 counts as strings.
func rejectedRecords(payload []byte) int64 {
	var response struct {
		PartialSuccess map[string]any `json:"partialSuccess"`
	}
	if json.Unmarshal(payload, &response) != nil {
		return 0
	}
	var total int64
	for key, value := range response.PartialSuccess {
		if !strings.HasPrefix(key, "rejected") {
			continue
		}
		switch count := value.(type) {
		case string:
			parsed, _ := strconv.ParseInt(count, 10, 64)
			total += parsed
		case float64:
			total += int64(count)
		}
	}
	return total
}
