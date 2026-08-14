package otlp

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/flight-observability/flight/internal/config"
	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type Server struct {
	grpcAddress     string
	httpAddress     string
	grpcServer      *grpc.Server
	httpServer      *http.Server
	receiver        *Receiver
	maxRequestBytes int64
	shutdownTimeout time.Duration
	ready           atomic.Bool
}

func NewServer(configuration config.Config, receiver *Receiver) *Server {
	grpcServer := grpc.NewServer(grpc.MaxRecvMsgSize(int(configuration.Ingestion.MaxRequestBytes)))
	collectortracepb.RegisterTraceServiceServer(grpcServer, &traceService{receiver: receiver})
	collectorlogspb.RegisterLogsServiceServer(grpcServer, &logsService{receiver: receiver})

	server := &Server{
		grpcAddress:     configuration.Ingestion.GRPCAddress,
		httpAddress:     configuration.Ingestion.HTTPAddress,
		grpcServer:      grpcServer,
		receiver:        receiver,
		maxRequestBytes: configuration.Ingestion.MaxRequestBytes,
		shutdownTimeout: configuration.ShutdownTimeout,
	}
	router := http.NewServeMux()
	router.HandleFunc("POST /v1/traces", server.traces)
	router.HandleFunc("POST /v1/logs", server.logs)
	server.httpServer = &http.Server{
		Addr:              server.httpAddress,
		Handler:           router,
		ReadHeaderTimeout: configuration.HTTP.ReadHeaderTimeout,
		ReadTimeout:       configuration.HTTP.ReadTimeout,
		WriteTimeout:      configuration.HTTP.WriteTimeout,
		IdleTimeout:       configuration.HTTP.IdleTimeout,
		MaxHeaderBytes:    configuration.HTTP.MaxHeaderBytes,
	}
	return server
}

func (server *Server) Run(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil
	}
	grpcListener, grpcListenError := net.Listen("tcp", server.grpcAddress)
	if grpcListenError != nil {
		return grpcListenError
	}
	httpListener, httpListenError := net.Listen("tcp", server.httpAddress)
	if httpListenError != nil {
		_ = grpcListener.Close()
		return httpListenError
	}
	server.ready.Store(true)
	defer server.ready.Store(false)

	results := make(chan error, 2)
	go func() { results <- server.grpcServer.Serve(grpcListener) }()
	go func() { results <- server.httpServer.Serve(httpListener) }()

	var runError error
	consumed := 0
	select {
	case runError = <-results:
		consumed = 1
	case <-ctx.Done():
	}
	server.ready.Store(false)
	shutdownError := server.shutdown()
	for consumed < 2 {
		serveError := <-results
		if !errors.Is(serveError, grpc.ErrServerStopped) && !errors.Is(serveError, http.ErrServerClosed) {
			runError = errors.Join(runError, serveError)
		}
		consumed++
	}
	return errors.Join(runError, shutdownError)
}

func (server *Server) Ready() bool {
	return server.ready.Load()
}

func (server *Server) shutdown() error {
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), server.shutdownTimeout)
	defer cancelShutdown()
	httpResult := make(chan error, 1)
	go func() { httpResult <- server.httpServer.Shutdown(shutdownContext) }()
	grpcDone := make(chan struct{})
	go func() {
		server.grpcServer.GracefulStop()
		close(grpcDone)
	}()

	var shutdownError error
	select {
	case shutdownError = <-httpResult:
	case <-shutdownContext.Done():
		shutdownError = shutdownContext.Err()
		_ = server.httpServer.Close()
	}
	select {
	case <-grpcDone:
	case <-shutdownContext.Done():
		server.grpcServer.Stop()
		shutdownError = errors.Join(shutdownError, shutdownContext.Err())
	}
	return shutdownError
}

func (server *Server) traces(responseWriter http.ResponseWriter, request *http.Request) {
	message := &collectortracepb.ExportTraceServiceRequest{}
	encoding, decodeError := server.decodeHTTP(responseWriter, request, message)
	if decodeError != nil {
		return
	}
	response, exportError := server.receiver.exportTraces(request.Context(), message)
	server.writeHTTP(responseWriter, encoding, response, exportError)
}

func (server *Server) logs(responseWriter http.ResponseWriter, request *http.Request) {
	message := &collectorlogspb.ExportLogsServiceRequest{}
	encoding, decodeError := server.decodeHTTP(responseWriter, request, message)
	if decodeError != nil {
		return
	}
	response, exportError := server.receiver.exportLogs(request.Context(), message)
	server.writeHTTP(responseWriter, encoding, response, exportError)
}

func (server *Server) decodeHTTP(responseWriter http.ResponseWriter, request *http.Request, message proto.Message) (string, error) {
	mediaType, _, parseError := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if parseError != nil || (mediaType != "application/x-protobuf" && mediaType != "application/json") {
		http.Error(responseWriter, "unsupported OTLP content type", http.StatusUnsupportedMediaType)
		return "", errors.New("unsupported content type")
	}

	limitedBody := http.MaxBytesReader(responseWriter, request.Body, server.maxRequestBytes)
	var reader io.Reader = limitedBody
	if contentEncoding := strings.ToLower(strings.TrimSpace(request.Header.Get("Content-Encoding"))); contentEncoding != "" && contentEncoding != "identity" {
		if contentEncoding != "gzip" {
			http.Error(responseWriter, "unsupported content encoding", http.StatusUnsupportedMediaType)
			return "", errors.New("unsupported content encoding")
		}
		gzipReader, gzipError := gzip.NewReader(limitedBody)
		if gzipError != nil {
			http.Error(responseWriter, "invalid gzip body", http.StatusBadRequest)
			return "", gzipError
		}
		defer gzipReader.Close()
		reader = gzipReader
	}
	body, readError := io.ReadAll(io.LimitReader(reader, server.maxRequestBytes+1))
	if readError != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(readError, &maxBytesError) {
			http.Error(responseWriter, "OTLP request body too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(responseWriter, "invalid OTLP request body", http.StatusBadRequest)
		}
		return "", readError
	}
	if int64(len(body)) > server.maxRequestBytes {
		http.Error(responseWriter, "OTLP request body too large", http.StatusRequestEntityTooLarge)
		return "", errors.New("request body too large")
	}
	if mediaType == "application/json" {
		if unmarshalError := protojson.Unmarshal(body, message); unmarshalError != nil {
			http.Error(responseWriter, "invalid OTLP JSON", http.StatusBadRequest)
			return "", unmarshalError
		}
	} else if unmarshalError := proto.Unmarshal(body, message); unmarshalError != nil {
		http.Error(responseWriter, "invalid OTLP protobuf", http.StatusBadRequest)
		return "", unmarshalError
	}
	return mediaType, nil
}

func (server *Server) writeHTTP(responseWriter http.ResponseWriter, encoding string, message proto.Message, exportError error) {
	if exportError != nil {
		http.Error(responseWriter, "OTLP ingestion temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	var body []byte
	var marshalError error
	if encoding == "application/json" {
		body, marshalError = protojson.Marshal(message)
	} else {
		body, marshalError = proto.Marshal(message)
	}
	if marshalError != nil {
		http.Error(responseWriter, "failed to encode OTLP response", http.StatusInternalServerError)
		return
	}
	responseWriter.Header().Set("Content-Type", encoding)
	responseWriter.WriteHeader(http.StatusOK)
	_, _ = responseWriter.Write(body)
}
