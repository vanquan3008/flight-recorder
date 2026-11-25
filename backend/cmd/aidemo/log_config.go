package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func configuredLogExporter(logger *slog.Logger) (*logExporter, error) {
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"))
	if endpoint == "" {
		return nil, nil
	}
	rawHeaders, present := os.LookupEnv("OTEL_EXPORTER_OTLP_LOGS_HEADERS")
	if !present {
		rawHeaders = os.Getenv("OTEL_EXPORTER_OTLP_HEADERS")
	}
	return newLogExporter(endpoint, rawHeaders, logger)
}

func newLogExporter(endpoint, rawHeaders string, logger *slog.Logger) (*logExporter, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, errors.New("invalid OTLP logs endpoint")
	}
	headers, err := parseLogHeaders(rawHeaders)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 2 * time.Second, Transport: http.DefaultTransport.(*http.Transport).Clone(), CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	ctx, cancel := context.WithCancel(context.Background())
	exporter := &logExporter{endpoint: parsed.String(), headers: headers, client: client, logger: logger,
		queue: make(chan []byte, logQueueSize), done: make(chan struct{}), cancel: cancel}
	go exporter.run(ctx)
	return exporter, nil
}

func parseLogHeaders(raw string) (http.Header, error) {
	headers := make(http.Header)
	if strings.TrimSpace(raw) == "" {
		return headers, nil
	}
	for _, pair := range strings.Split(raw, ",") {
		name, value, found := strings.Cut(pair, "=")
		name = strings.TrimSpace(name)
		decoded, err := url.PathUnescape(strings.TrimSpace(value))
		if !found || err != nil || name == "" ||
			strings.ContainsFunc(name, func(r rune) bool {
				return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
					strings.ContainsRune("!#$%&'*+-.^_`|~", r))
			}) ||
			strings.ContainsFunc(decoded, func(r rune) bool { return (r < ' ' && r != '	') || r == 127 }) {
			return nil, errors.New("invalid OTLP logs headers")
		}
		headers.Set(name, decoded)
	}
	return headers, nil
}
