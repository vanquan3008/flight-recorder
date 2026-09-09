package otlp

import (
	"compress/gzip"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func newHTTPServer(configuration config.Config, server *Server) *http.Server {
	router := http.NewServeMux()
	router.HandleFunc("POST /v1/traces", server.traces)
	router.HandleFunc("POST /v1/logs", server.logs)
	router.HandleFunc("POST /v1/metrics", server.httpMetrics)
	return &http.Server{
		Addr:              server.httpAddress,
		Handler:           router,
		ReadHeaderTimeout: configuration.HTTP.ReadHeaderTimeout,
		ReadTimeout:       configuration.HTTP.ReadTimeout,
		WriteTimeout:      configuration.HTTP.WriteTimeout,
		IdleTimeout:       configuration.HTTP.IdleTimeout,
		MaxHeaderBytes:    configuration.HTTP.MaxHeaderBytes,
	}
}

func (server *Server) traces(responseWriter http.ResponseWriter, request *http.Request) {
	startedAt, outcome := time.Now(), "ok"
	defer func() { server.observeHTTPRequest("traces", outcome, time.Since(startedAt)) }()
	message := &collectortracepb.ExportTraceServiceRequest{}
	encoding, decodeError := server.decodeHTTP(responseWriter, request, message)
	if decodeError != nil {
		outcome = "rejected"
		return
	}
	if authError := server.receiver.authorize(request.Context(), httpAPIKey(request)); authError != nil {
		outcome = "unauthenticated"
		server.writeHTTPError(responseWriter, encoding, http.StatusUnauthorized, "OTLP authentication failed")
		return
	}
	response, exportError := server.receiver.exportTraces(request.Context(), message)
	if exportError != nil {
		outcome = "unavailable"
	} else if response.GetPartialSuccess() != nil {
		outcome = "partial"
	}
	server.writeHTTP(responseWriter, encoding, response, exportError)
}

func (server *Server) logs(responseWriter http.ResponseWriter, request *http.Request) {
	startedAt, outcome := time.Now(), "ok"
	defer func() { server.observeHTTPRequest("logs", outcome, time.Since(startedAt)) }()
	message := &collectorlogspb.ExportLogsServiceRequest{}
	encoding, decodeError := server.decodeHTTP(responseWriter, request, message)
	if decodeError != nil {
		outcome = "rejected"
		return
	}
	if authError := server.receiver.authorize(request.Context(), httpAPIKey(request)); authError != nil {
		outcome = "unauthenticated"
		server.writeHTTPError(responseWriter, encoding, http.StatusUnauthorized, "OTLP authentication failed")
		return
	}
	response, exportError := server.receiver.exportLogs(request.Context(), message)
	if exportError != nil {
		outcome = "unavailable"
	} else if response.GetPartialSuccess() != nil {
		outcome = "partial"
	}
	server.writeHTTP(responseWriter, encoding, response, exportError)
}

func (server *Server) httpMetrics(responseWriter http.ResponseWriter, request *http.Request) {
	startedAt, outcome := time.Now(), "ok"
	defer func() { server.observeHTTPRequest("metrics", outcome, time.Since(startedAt)) }()
	message := &collectormetricspb.ExportMetricsServiceRequest{}
	encoding, decodeError := server.decodeHTTP(responseWriter, request, message)
	if decodeError != nil {
		outcome = "rejected"
		return
	}
	if authError := server.receiver.authorize(request.Context(), httpAPIKey(request)); authError != nil {
		outcome = "unauthenticated"
		server.writeHTTPError(responseWriter, encoding, http.StatusUnauthorized, "OTLP authentication failed")
		return
	}
	response, exportError := server.receiver.exportMetrics(request.Context(), message)
	if exportError != nil {
		outcome = "unavailable"
	} else if response.GetPartialSuccess() != nil {
		outcome = "partial"
	}
	server.writeHTTP(responseWriter, encoding, response, exportError)
}

func (server *Server) observeHTTPRequest(signal, outcome string, duration time.Duration) {
	if server.receiver != nil && server.receiver.metrics != nil {
		server.receiver.metrics.ObserveOTLPRequest("http", signal, outcome, duration)
	}
}

func (server *Server) decodeHTTP(responseWriter http.ResponseWriter, request *http.Request, message proto.Message) (string, error) {
	encoding := responseEncoding(request)
	mediaType, _, parseError := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if parseError != nil || (mediaType != "application/x-protobuf" && mediaType != "application/json") {
		server.writeHTTPError(responseWriter, encoding, http.StatusUnsupportedMediaType, "unsupported OTLP content type")
		return encoding, errors.New("unsupported content type")
	}

	limitedBody := http.MaxBytesReader(responseWriter, request.Body, server.maxRequestBytes)
	var reader io.Reader = limitedBody
	if contentEncoding := strings.ToLower(strings.TrimSpace(request.Header.Get("Content-Encoding"))); contentEncoding != "" && contentEncoding != "identity" {
		if contentEncoding != "gzip" {
			server.writeHTTPError(responseWriter, encoding, http.StatusUnsupportedMediaType, "unsupported OTLP content encoding")
			return encoding, errors.New("unsupported content encoding")
		}
		gzipReader, gzipError := gzip.NewReader(limitedBody)
		if gzipError != nil {
			server.writeHTTPError(responseWriter, encoding, http.StatusBadRequest, "invalid gzip body")
			return encoding, gzipError
		}
		defer gzipReader.Close()
		reader = gzipReader
	}
	body, readError := io.ReadAll(io.LimitReader(reader, server.maxRequestBytes+1))
	if readError != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(readError, &maxBytesError) {
			server.writeHTTPError(responseWriter, encoding, http.StatusRequestEntityTooLarge, "OTLP request body too large")
		} else {
			server.writeHTTPError(responseWriter, encoding, http.StatusBadRequest, "invalid OTLP request body")
		}
		return encoding, readError
	}
	if int64(len(body)) > server.maxRequestBytes {
		server.writeHTTPError(responseWriter, encoding, http.StatusRequestEntityTooLarge, "OTLP request body too large")
		return encoding, errors.New("request body too large")
	}
	if mediaType == "application/json" {
		if unmarshalError := protojson.Unmarshal(body, message); unmarshalError != nil {
			server.writeHTTPError(responseWriter, encoding, http.StatusBadRequest, "invalid OTLP JSON")
			return encoding, unmarshalError
		}
	} else if unmarshalError := proto.Unmarshal(body, message); unmarshalError != nil {
		server.writeHTTPError(responseWriter, encoding, http.StatusBadRequest, "invalid OTLP protobuf")
		return encoding, unmarshalError
	}
	return encoding, nil
}

func (server *Server) writeHTTP(responseWriter http.ResponseWriter, encoding string, message proto.Message, exportError error) {
	if exportError != nil {
		server.writeHTTPError(responseWriter, encoding, http.StatusServiceUnavailable, "OTLP ingestion temporarily unavailable")
		return
	}
	body, marshalError := marshalHTTPMessage(encoding, message)
	if marshalError != nil {
		http.Error(responseWriter, "failed to encode OTLP response", http.StatusInternalServerError)
		return
	}
	responseWriter.Header().Set("Content-Type", encoding)
	responseWriter.WriteHeader(http.StatusOK)
	_, _ = responseWriter.Write(body)
}

func (server *Server) writeHTTPError(responseWriter http.ResponseWriter, encoding string, statusCode int, message string) {
	body, marshalError := marshalHTTPMessage(encoding, &statuspb.Status{Message: message})
	if marshalError != nil {
		http.Error(responseWriter, "failed to encode OTLP error", http.StatusInternalServerError)
		return
	}
	if statusCode == http.StatusServiceUnavailable {
		responseWriter.Header().Set("Retry-After", "1")
	}
	responseWriter.Header().Set("Content-Type", encoding)
	responseWriter.WriteHeader(statusCode)
	_, _ = responseWriter.Write(body)
}

func responseEncoding(request *http.Request) string {
	mediaType, _, parseError := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if parseError == nil && mediaType == "application/json" {
		return "application/json"
	}
	return "application/x-protobuf"
}

func httpAPIKey(request *http.Request) string {
	if rawKey := strings.TrimSpace(request.Header.Get("X-API-Key")); rawKey != "" {
		return rawKey
	}
	if authorization := request.Header.Get("Authorization"); strings.HasPrefix(authorization, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
	}
	return ""
}

func marshalHTTPMessage(encoding string, message proto.Message) ([]byte, error) {
	if encoding == "application/json" {
		return protojson.Marshal(message)
	}
	return proto.Marshal(message)
}
