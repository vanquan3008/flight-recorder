package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"regexp"
	"time"
)

const corsMaxAgeSeconds = "600"

const requestIDHeader = "X-Request-ID"

var safeRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

type responseStatusWriter struct {
	http.ResponseWriter
	statusCode int
	headerSent bool
}

func (writer *responseStatusWriter) WriteHeader(statusCode int) {
	if writer.headerSent {
		return
	}
	writer.headerSent = true
	writer.statusCode = statusCode
	writer.ResponseWriter.WriteHeader(statusCode)
}

func (writer *responseStatusWriter) Unwrap() http.ResponseWriter {
	return writer.ResponseWriter
}

func (server *Server) withAccessLog(nextHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		requestStartedAt := time.Now()
		statusWriter := &responseStatusWriter{ResponseWriter: responseWriter, statusCode: http.StatusOK}
		nextHandler.ServeHTTP(statusWriter, request)
		server.logger.Info("http request",
			"request_id", request.Header.Get(requestIDHeader),
			"method", request.Method,
			"path", request.URL.Path,
			"status", statusWriter.statusCode,
			"duration_ms", time.Since(requestStartedAt).Milliseconds(),
		)
	})
}

func (server *Server) withMetrics(nextHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		requestStartedAt := time.Now()
		statusWriter := &responseStatusWriter{ResponseWriter: responseWriter, statusCode: http.StatusOK}
		nextHandler.ServeHTTP(statusWriter, request)
		route := request.Pattern
		if route == "" {
			route = "unmatched"
		}
		server.metrics.ObserveHTTPRequest(metricMethod(request.Method), route, statusWriter.statusCode, time.Since(requestStartedAt))
	})
}

func metricMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return method
	default:
		return "OTHER"
	}
}

func (server *Server) withBodyLimit(nextHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		request.Body = http.MaxBytesReader(responseWriter, request.Body, server.maxBodyBytes)
		nextHandler.ServeHTTP(responseWriter, request)
	})
}

func (server *Server) withRequestID(nextHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		requestID := request.Header.Get(requestIDHeader)
		if !safeRequestID.MatchString(requestID) {
			requestID = newRequestID()
		}
		request.Header.Set(requestIDHeader, requestID)
		responseWriter.Header().Set(requestIDHeader, requestID)
		nextHandler.ServeHTTP(responseWriter, request)
	})
}

func (server *Server) withCORS(nextHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get("Origin")
		if origin == "" {
			nextHandler.ServeHTTP(responseWriter, request)
			return
		}
		if _, allowed := server.allowedOrigins[origin]; !allowed {
			if request.Method == http.MethodOptions {
				http.Error(responseWriter, "origin is not allowed", http.StatusForbidden)
				return
			}
			nextHandler.ServeHTTP(responseWriter, request)
			return
		}

		responseWriter.Header().Set("Access-Control-Allow-Origin", origin)
		responseWriter.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-API-Key, X-Request-ID")
		responseWriter.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PATCH, DELETE, OPTIONS")
		responseWriter.Header().Set("Access-Control-Max-Age", corsMaxAgeSeconds)
		responseWriter.Header().Add("Vary", "Origin")
		if request.Method == http.MethodOptions {
			responseWriter.WriteHeader(http.StatusNoContent)
			return
		}
		nextHandler.ServeHTTP(responseWriter, request)
	})
}

func (server *Server) withPanicRecovery(nextHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		defer func() {
			if recoveredValue := recover(); recoveredValue != nil {
				server.logger.Error("http panic", "request_id", request.Header.Get(requestIDHeader), "error", recoveredValue)
				writeJSON(responseWriter, http.StatusInternalServerError, `{"error":{"code":"INTERNAL_ERROR","message":"internal server error"}}`)
			}
		}()
		nextHandler.ServeHTTP(responseWriter, request)
	})
}

func newRequestID() string {
	var randomBytes [16]byte
	if _, randomError := rand.Read(randomBytes[:]); randomError != nil {
		timestamp := time.Now().UTC().Format(time.RFC3339Nano)
		return hex.EncodeToString([]byte(timestamp))
	}
	return hex.EncodeToString(randomBytes[:])
}
