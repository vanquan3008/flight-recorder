package otlp

import (
	"compress/gzip"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/vanquan3008/tracepilot/internal/auth"
	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/middleware"
)

func newHTTPServer(configuration config.Config, server *Server) *http.Server {
	router := http.NewServeMux()
	router.HandleFunc("POST /v1/traces", handleHTTP(server, "traces", func() *collectortracepb.ExportTraceServiceRequest {
		return &collectortracepb.ExportTraceServiceRequest{}
	}, server.receiver.exportTraces))
	router.HandleFunc("POST /v1/logs", handleHTTP(server, "logs", func() *collectorlogspb.ExportLogsServiceRequest { return &collectorlogspb.ExportLogsServiceRequest{} }, server.receiver.exportLogs))
	router.HandleFunc("POST /v1/metrics", handleHTTP(server, "metrics", func() *collectormetricspb.ExportMetricsServiceRequest {
		return &collectormetricspb.ExportMetricsServiceRequest{}
	}, server.receiver.exportMetrics))
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

func (server *Server) observeHTTPRequest(signal, outcome string, duration time.Duration) {
	if server.receiver != nil && server.receiver.metrics != nil {
		server.receiver.metrics.ObserveOTLPRequest("http", signal, outcome, duration)
	}
}

func (server *Server) authorizeHTTP(responseWriter http.ResponseWriter, request *http.Request) (*http.Request, string) {
	scoped, err := server.receiver.authorizeScope(request.Context(), httpToken(request), strings.TrimSpace(request.Header.Get("X-Environment")), server.receiver.clientAddress(request.RemoteAddr, request.Header.Values("X-Forwarded-For")))
	if err == nil {
		return request.WithContext(scoped), ""
	}
	var throttled *throttledError
	if errors.As(err, &throttled) {
		responseWriter.Header().Set("Retry-After", middleware.RetryAfterSeconds(throttled.retryAfter))
		server.writeHTTPError(responseWriter, responseEncoding(request), http.StatusTooManyRequests, "too many failed authentication attempts")
		return request, "throttled"
	}
	if errors.Is(err, auth.ErrUnauthorized) {
		server.writeHTTPError(responseWriter, responseEncoding(request), http.StatusUnauthorized, "OTLP authentication failed")
		return request, "unauthenticated"
	}
	if errors.Is(err, ErrUnknownEnvironment) {
		server.writeHTTPError(responseWriter, responseEncoding(request), http.StatusBadRequest, "unknown environment for this project")
		return request, "rejected"
	}
	server.writeHTTPError(responseWriter, responseEncoding(request), http.StatusServiceUnavailable, "OTLP authentication temporarily unavailable")
	return request, "unavailable"
}

// admitHTTP runs after authentication and before the body is read, so an
// overloaded receiver answers 503 without buffering another payload.
func (server *Server) admitHTTP(responseWriter http.ResponseWriter, request *http.Request) (func(), string) {
	release, admitted := server.receiver.admit()
	if admitted {
		return release, ""
	}
	server.writeHTTPError(responseWriter, responseEncoding(request), http.StatusServiceUnavailable, "OTLP receiver is at its concurrent request limit")
	return nil, "overloaded"
}

var gzipReaderPool sync.Pool

func getGzipReader(reader io.Reader) (*gzip.Reader, error) {
	if cached := gzipReaderPool.Get(); cached != nil {
		gzipReader := cached.(*gzip.Reader)
		if resetError := gzipReader.Reset(reader); resetError == nil {
			return gzipReader, nil
		}
	}
	return gzip.NewReader(reader)
}

func putGzipReader(gzipReader *gzip.Reader) {
	_ = gzipReader.Close()
	gzipReaderPool.Put(gzipReader)
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
		gzipReader, gzipError := getGzipReader(limitedBody)
		if gzipError != nil {
			server.writeHTTPError(responseWriter, encoding, http.StatusBadRequest, "invalid gzip body")
			return encoding, gzipError
		}
		defer putGzipReader(gzipReader)
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
		if unmarshalError := unmarshalOTLPJSON(body, message); unmarshalError != nil {
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

// httpToken returns the ingest token a sender presents as a bearer credential.
func httpToken(request *http.Request) string {
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
