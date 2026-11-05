// Package clickhouse stores raw application logs using ClickHouse's HTTP interface.
package clickhouse

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
)

const maxResponseBytes = 32 << 20

type Store struct {
	endpoint *url.URL
	client   *http.Client
	cfg      config.ClickHouseConfig
	maxAge   time.Duration
}

func New(cfg config.ClickHouseConfig, maxAge time.Duration) (*Store, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.URL == "" || maxAge < 0 {
		return nil, errors.New("invalid ClickHouse resource limits")
	}
	endpoint, _ := url.Parse(cfg.URL)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxConnsPerHost, transport.MaxIdleConnsPerHost = cfg.MaxConnections, cfg.MaxConnections
	return &Store{
		endpoint: endpoint, cfg: cfg, maxAge: maxAge,
		client: &http.Client{Transport: transport, Timeout: cfg.RequestTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

func (store *Store) Close() { store.client.CloseIdleConnections() }

// Parameters use ClickHouse's escaped format, then normal URL encoding.
func parameter(value string) string {
	return strings.NewReplacer("\\", "\\\\", "\t", "\\t", "\n", "\\n", "\r", "\\r", "\x00", "\\0").Replace(value)
}

func (store *Store) execute(ctx context.Context, sql string, parameters map[string]string, payload []byte, insert bool) ([]byte, error) {
	endpoint := *store.endpoint
	values := url.Values{"query": {sql}, "database": {store.cfg.Database},
		"date_time_input_format": {"best_effort"}, "date_time_output_format": {"iso"},
		"output_format_json_quote_64bit_integers": {"0"}, "wait_end_of_query": {"1"},
		"max_execution_time": {strconv.Itoa(max(1, int(store.cfg.RequestTimeout.Seconds())))}}
	if strings.HasPrefix(sql, "CREATE DATABASE ") {
		values.Set("database", "default")
	}
	if strings.HasPrefix(sql, "ALTER TABLE ") {
		values.Set("materialize_ttl_after_modify", "0")
	}
	for key, value := range parameters {
		values.Set("param_"+key, parameter(value))
	}
	if insert {
		values.Set("async_insert", "1")
		values.Set("wait_for_async_insert", "1")
	}
	endpoint.RawQuery = values.Encode()
	var body io.Reader
	if len(payload) > 0 {
		var compressed bytes.Buffer
		writer, _ := gzip.NewWriterLevel(&compressed, gzip.BestSpeed)
		if _, err := writer.Write(payload); err != nil {
			return nil, err
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
		body = &compressed
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), body)
	if err != nil {
		return nil, errors.New("build ClickHouse request")
	}
	request.Header.Set("X-ClickHouse-User", store.cfg.Username)
	request.Header.Set("X-ClickHouse-Key", store.cfg.Password)
	if body != nil {
		request.Header.Set("Content-Encoding", "gzip")
	}
	response, err := store.client.Do(request)
	if err != nil {
		// Keep endpoint, credentials, filters and telemetry out of error logs.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, context.DeadlineExceeded
		}
		return nil, errors.New("ClickHouse request failed")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, errors.New("read ClickHouse response")
	}
	if len(data) > maxResponseBytes {
		return nil, errors.New("ClickHouse result exceeds 32 MiB; narrow the log query")
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("X-ClickHouse-Exception-Code") != "" {
		return nil, fmt.Errorf("ClickHouse query failed (HTTP %d)", response.StatusCode)
	}
	if insert && len(bytes.TrimSpace(data)) != 0 {
		return nil, errors.New("ClickHouse did not confirm the insert")
	}
	return data, nil
}
