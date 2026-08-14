package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
)

func main() {
	applicationContext, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	exporter, exporterError := otlptracehttp.New(applicationContext)
	if exporterError != nil {
		log.Fatal(exporterError)
	}
	tracerProvider := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(resource.NewSchemaless(attribute.String("service.name", "flight-demo"))),
	)
	defer func() {
		shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelShutdown()
		_ = tracerProvider.Shutdown(shutdownContext)
	}()
	otel.SetTracerProvider(tracerProvider)
	tracer := tracerProvider.Tracer("flight-demo")

	router := http.NewServeMux()
	router.HandleFunc("GET /", func(responseWriter http.ResponseWriter, request *http.Request) {
		_, span := tracer.Start(request.Context(), "GET /")
		defer span.End()
		span.SetAttributes(attribute.Int("http.response.status_code", http.StatusOK))
		_, _ = responseWriter.Write([]byte("flight demo\n"))
	})
	router.HandleFunc("GET /error", func(responseWriter http.ResponseWriter, request *http.Request) {
		_, span := tracer.Start(request.Context(), "GET /error")
		defer span.End()
		demoError := errors.New("synthetic checkout failure")
		span.RecordError(demoError)
		span.SetStatus(codes.Error, demoError.Error())
		span.SetAttributes(attribute.Int("http.response.status_code", http.StatusInternalServerError))
		http.Error(responseWriter, demoError.Error(), http.StatusInternalServerError)
	})

	server := &http.Server{Addr: ":8090", Handler: router, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-applicationContext.Done()
		shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelShutdown()
		_ = server.Shutdown(shutdownContext)
	}()
	if serveError := server.ListenAndServe(); serveError != nil && !errors.Is(serveError, http.ErrServerClosed) {
		log.Fatal(serveError)
	}
}
