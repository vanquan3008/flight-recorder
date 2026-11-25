// Package client speaks ClickHouse's HTTP interface and holds the query helpers every ClickHouse
// store shares: parameter binding, row decoding, scope validation, and value encoding.
package client

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

	"github.com/vanquan3008/tracepilot/internal/config"
)

// MaxResponseBytes caps one buffered result, so a wide query cannot exhaust memory.
const MaxResponseBytes = 32 << 20

// Client sends statements to one ClickHouse server over HTTP.
type Client struct {
	endpoint *url.URL
	// HTTP is the underlying client; tests replace its timeout and transport.
	HTTP *http.Client
	cfg  config.ClickHouseConfig
}

func New(cfg config.ClickHouseConfig) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.URL == "" {
		return nil, errors.New("invalid ClickHouse resource limits")
	}
	endpoint, _ := url.Parse(cfg.URL)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxConnsPerHost, transport.MaxIdleConnsPerHost = cfg.MaxConnections, cfg.MaxConnections
	return &Client{
		endpoint: endpoint, cfg: cfg,
		HTTP: &http.Client{Transport: transport, Timeout: cfg.RequestTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

// Config is the configuration the client was built from.
func (client *Client) Config() config.ClickHouseConfig { return client.cfg }

func (client *Client) Close() { client.HTTP.CloseIdleConnections() }

// Parameter escapes a bound value in ClickHouse's format; the URL encoding comes after.
func Parameter(value string) string {
	return strings.NewReplacer("\\", "\\\\", "\t", "\\t", "\n", "\\n", "\r", "\\r", "\x00", "\\0").Replace(value)
}

// Execute runs a statement and returns its whole response, which must fit in MaxResponseBytes.
func (client *Client) Execute(ctx context.Context, sql string, parameters map[string]string, payload []byte, insert bool) ([]byte, error) {
	response, err := client.OpenResponse(ctx, sql, parameters, payload, insert)
	if err != nil {
		return nil, err
	}
	defer response.Close()
	data, err := io.ReadAll(io.LimitReader(response, MaxResponseBytes+1))
	if err != nil {
		return nil, ResponseError(ctx, err, "read ClickHouse response")
	}
	if len(data) > MaxResponseBytes {
		return nil, errors.New("ClickHouse result exceeds 32 MiB; narrow the log query")
	}
	if insert && len(bytes.TrimSpace(data)) != 0 {
		return nil, errors.New("ClickHouse did not confirm the insert")
	}
	return data, nil
}

// OpenResponse returns a successful response body. The caller must close it.
func (client *Client) OpenResponse(ctx context.Context, sql string, parameters map[string]string, payload []byte, insert bool) (io.ReadCloser, error) {
	endpoint := *client.endpoint
	values := url.Values{"query": {sql}, "database": {client.cfg.Database},
		"date_time_input_format": {"best_effort"}, "date_time_output_format": {"iso"},
		"output_format_json_quote_64bit_integers": {"0"}, "wait_end_of_query": {"1"},
		"max_execution_time": {strconv.Itoa(max(1, int(client.cfg.RequestTimeout.Seconds())))}}
	if strings.HasPrefix(sql, "CREATE DATABASE ") || strings.Contains(sql, "FROM system.tables") {
		values.Set("database", "default")
	}
	if strings.HasPrefix(sql, "ALTER TABLE ") {
		values.Set("materialize_ttl_after_modify", "0")
	}
	for key, value := range parameters {
		values.Set("param_"+key, Parameter(value))
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
	request.Header.Set("X-ClickHouse-User", client.cfg.Username)
	request.Header.Set("X-ClickHouse-Key", client.cfg.Password)
	if body != nil {
		request.Header.Set("Content-Encoding", "gzip")
	}
	response, err := client.HTTP.Do(request)
	if err != nil {
		return nil, ResponseError(ctx, err, "ClickHouse request failed")
	}
	if code := response.Header.Get("X-ClickHouse-Exception-Code"); response.StatusCode != http.StatusOK || code != "" {
		response.Body.Close()
		return nil, queryFailure(response.StatusCode, code)
	}
	return response.Body, nil
}

// queryFailure names the ClickHouse exception code, which tells an operator
// what failed (for example 241 for memory, 159 for timeout) without echoing the
// response body, which may quote telemetry or SQL.
func queryFailure(status int, code string) error {
	if code != "" && len(code) <= 6 && strings.Trim(code, "0123456789") == "" {
		return fmt.Errorf("ClickHouse query failed (HTTP %d, code %s)", status, code)
	}
	return fmt.Errorf("ClickHouse query failed (HTTP %d)", status)
}

// ResponseError reduces a transport failure to its cause, keeping the endpoint, credentials,
// filters and telemetry out of logs.
func ResponseError(ctx context.Context, err error, message string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return errors.New(message)
}
