package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	logQueueSize     = 256
	logBatchSize     = 32
	logBatchBytes    = 32 << 10
	logRecordBytes   = 1 << 10
	logFlushInterval = 250 * time.Millisecond
	logBatchTimeout  = 5 * time.Second
	logBatchPrefix   = `{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"` + serviceName + `"}}]},"scopeLogs":[{"scope":{"name":"` + serviceName + `"},"logRecords":[`
	logBatchSuffix   = "]}]}]}"
)

type logValue struct {
	String string `json:"stringValue,omitempty"`
	Int    string `json:"intValue,omitempty"`
	Bool   *bool  `json:"boolValue,omitempty"`
}

type logAttribute struct {
	Key   string   `json:"key"`
	Value logValue `json:"value"`
}

type logRecord struct {
	Time           string         `json:"timeUnixNano"`
	SeverityNumber int            `json:"severityNumber"`
	SeverityText   string         `json:"severityText"`
	TraceID        string         `json:"traceId"`
	SpanID         string         `json:"spanId"`
	Body           logValue       `json:"body"`
	Attributes     []logAttribute `json:"attributes,omitempty"`
}

type logExporter struct {
	endpoint                                     string
	headers                                      http.Header
	client                                       *http.Client
	logger                                       *slog.Logger
	queue                                        chan []byte
	done                                         chan struct{}
	cancel                                       context.CancelFunc
	mu                                           sync.RWMutex
	closed                                       bool
	dropped, accepted, retries, rejected, failed atomic.Int64
}

func (exporter *logExporter) enqueue(record logRecord) {
	payload, err := json.Marshal(record)
	if err != nil || len(payload) > logRecordBytes {
		exporter.dropped.Add(1)
		return
	}
	exporter.mu.RLock()
	defer exporter.mu.RUnlock()
	if !exporter.closed {
		select {
		case exporter.queue <- payload:
			return
		default:
		}
	}
	exporter.dropped.Add(1)
}

func (exporter *logExporter) shutdown(ctx context.Context) error {
	exporter.mu.Lock()
	if !exporter.closed {
		exporter.closed = true
		close(exporter.queue)
	}
	exporter.mu.Unlock()
	select {
	case <-exporter.done:
		return nil
	case <-ctx.Done():
		exporter.cancel()
		return ctx.Err()
	}
}

func (exporter *logExporter) run(ctx context.Context) {
	defer close(exporter.done)
	defer exporter.cancel()
	defer exporter.client.CloseIdleConnections()
	defer func() {
		exporter.logger.Info("OTLP log export stopped", "accepted_batches", exporter.accepted.Load(),
			"retries", exporter.retries.Load(), "rejected_records", exporter.rejected.Load(),
			"dropped_records", exporter.dropped.Load(), "failed_records", exporter.failed.Load())
	}()
	timer := time.NewTimer(logFlushInterval)
	timer.Stop()
	defer timer.Stop()
	var wait <-chan time.Time
	batch := make([][]byte, 0, logBatchSize)
	const envelope = len(logBatchPrefix) + len(logBatchSuffix)
	size := envelope
	flush := func() {
		timer.Stop()
		wait = nil
		if len(batch) > 0 {
			exporter.send(ctx, batch)
			clear(batch)
			batch, size = batch[:0], envelope
		}
	}
	for {
		if ctx.Err() != nil {
			exporter.dropped.Add(int64(len(batch) + len(exporter.queue)))
			return
		}
		select {
		case record, ok := <-exporter.queue:
			if !ok {
				flush()
				return
			}
			if size+len(record)+len(batch) > logBatchBytes {
				flush()
			}
			if len(batch) == 0 {
				timer.Reset(logFlushInterval)
				wait = timer.C
			}
			batch = append(batch, record)
			size += len(record)
			if len(batch) == logBatchSize {
				flush()
			}
		case <-wait:
			flush()
		case <-ctx.Done():
		}
	}
}

func (exporter *logExporter) send(parent context.Context, records [][]byte) {
	ctx, cancel := context.WithTimeout(parent, logBatchTimeout)
	defer cancel()
	payload := []byte(logBatchPrefix)
	payload = append(payload, bytes.Join(records, []byte(","))...)
	payload = append(payload, logBatchSuffix...)
	result := exporter.post(ctx, payload)
	if result.retry && ctx.Err() == nil {
		delay := retryDelay(result.retryAfter, time.Now())
		deadline, _ := ctx.Deadline()
		if delay < time.Until(deadline) {
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
				exporter.retries.Add(1)
				result = exporter.post(ctx, payload)
			case <-ctx.Done():
			}
			timer.Stop()
		}
	}
	if result.failure != "" {
		exporter.failed.Add(int64(len(records)))
		exporter.logger.Warn("OTLP log export failed", "error.type", result.failure, "records", len(records))
		return
	}
	exporter.accepted.Add(1)
	exporter.rejected.Add(result.rejected)
	if result.rejected > 0 {
		exporter.logger.Warn("OTLP log records rejected", "records", result.rejected)
	}
}

type logPostResult struct {
	failure, retryAfter string
	retry               bool
	rejected            int64
}

func (exporter *logExporter) post(ctx context.Context, payload []byte) logPostResult {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, exporter.endpoint, bytes.NewReader(payload))
	if err != nil {
		return logPostResult{failure: "invalid_request"}
	}
	request.Header = exporter.headers.Clone()
	request.Header.Set("Content-Type", "application/json")
	response, err := exporter.client.Do(request)
	if err != nil {
		return logPostResult{failure: "transport", retry: transientLogError(err)}
	}
	defer response.Body.Close()
	switch status := response.StatusCode; status {
	case http.StatusOK:
		// Read the export response below.
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return logPostResult{failure: "http_" + strconv.Itoa(status), retry: true, retryAfter: response.Header.Get("Retry-After")}
	default:
		return logPostResult{failure: "http_" + strconv.Itoa(status)}
	}
	const limit = 64 << 10
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	var output struct {
		PartialSuccess struct {
			Rejected json.Number `json:"rejectedLogRecords"`
		} `json:"partialSuccess"`
	}
	if err != nil || len(data) > limit || json.Unmarshal(data, &output) != nil {
		return logPostResult{failure: "invalid_response"}
	}
	if output.PartialSuccess.Rejected == "" {
		return logPostResult{}
	}
	rejected, err := output.PartialSuccess.Rejected.Int64()
	if err != nil || rejected < 0 {
		return logPostResult{failure: "invalid_response"}
	}
	return logPostResult{rejected: rejected}
}

func transientLogError(err error) bool {
	_, isOp := errors.AsType[*net.OpError](err)
	network, isNetwork := errors.AsType[net.Error](err)
	return errors.Is(err, io.EOF) || isOp || (isNetwork && network.Timeout())
}

func retryDelay(value string, now time.Time) time.Duration {
	if seconds, err := strconv.ParseUint(value, 10, 64); err == nil {
		return time.Duration(min(seconds, uint64(logBatchTimeout/time.Second))) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		return max(0, at.Sub(now))
	}
	return 100*time.Millisecond + rand.N(100*time.Millisecond)
}
