package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

const (
	CORSMaxAgeSeconds = "600"
	RequestIDHeader   = render.RequestIDHeader
)

var safeRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// HTTPObserver records request metrics.
type HTTPObserver interface {
	ObserveHTTPRequest(method, route string, status int, duration time.Duration)
}

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

// AccessLog records incoming HTTP requests with structured attributes.
func AccessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(nextHandler http.Handler) http.Handler {
		return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			requestStartedAt := time.Now()
			statusWriter := &responseStatusWriter{ResponseWriter: responseWriter, statusCode: http.StatusOK}
			nextHandler.ServeHTTP(statusWriter, request)
			if logger != nil {
				logger.Info("http request",
					"request_id", request.Header.Get(RequestIDHeader),
					"method", request.Method,
					"path", request.URL.Path,
					"status", statusWriter.statusCode,
					"duration_ms", time.Since(requestStartedAt).Milliseconds(),
				)
			}
		})
	}
}

// Metrics measures request duration and counts by status code and route pattern.
func Metrics(observer HTTPObserver) func(http.Handler) http.Handler {
	return func(nextHandler http.Handler) http.Handler {
		return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			requestStartedAt := time.Now()
			statusWriter := &responseStatusWriter{ResponseWriter: responseWriter, statusCode: http.StatusOK}
			nextHandler.ServeHTTP(statusWriter, request)
			route := request.Pattern
			if route == "" {
				route = "unmatched"
			}
			if observer != nil {
				observer.ObserveHTTPRequest(MetricMethod(request.Method), route, statusWriter.statusCode, time.Since(requestStartedAt))
			}
		})
	}
}

// MetricMethod bounds metric cardinality to standard HTTP verbs.
func MetricMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return method
	default:
		return "OTHER"
	}
}

// BodyLimit caps maximum allowable body size.
func BodyLimit(maxBodyBytes int64) func(http.Handler) http.Handler {
	return func(nextHandler http.Handler) http.Handler {
		return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			if maxBodyBytes > 0 {
				request.Body = http.MaxBytesReader(responseWriter, request.Body, maxBodyBytes)
			}
			nextHandler.ServeHTTP(responseWriter, request)
		})
	}
}

// RequestID ensures every request has a validated X-Request-ID header.
func RequestID() func(http.Handler) http.Handler {
	return func(nextHandler http.Handler) http.Handler {
		return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			requestID := request.Header.Get(RequestIDHeader)
			if !safeRequestID.MatchString(requestID) {
				requestID = NewRequestID()
			}
			request.Header.Set(RequestIDHeader, requestID)
			responseWriter.Header().Set(RequestIDHeader, requestID)
			nextHandler.ServeHTTP(responseWriter, request)
		})
	}
}

// SecurityHeaders writes standard security headers onto the response.
func SecurityHeaders(hsts bool) func(http.Handler) http.Handler {
	return func(nextHandler http.Handler) http.Handler {
		return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			responseWriter.Header().Set("X-Content-Type-Options", "nosniff")
			responseWriter.Header().Set("X-Frame-Options", "DENY")
			responseWriter.Header().Set("Referrer-Policy", "no-referrer")
			responseWriter.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
			if hsts {
				responseWriter.Header().Set("Strict-Transport-Security", "max-age=31536000")
			}
			nextHandler.ServeHTTP(responseWriter, request)
		})
	}
}

// CORS handles Cross-Origin Resource Sharing and preflight requests.
func CORS(allowedOrigins map[string]struct{}, allowedMethods string) func(http.Handler) http.Handler {
	return func(nextHandler http.Handler) http.Handler {
		return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			origin := request.Header.Get("Origin")
			if origin == "" {
				nextHandler.ServeHTTP(responseWriter, request)
				return
			}
			if _, allowed := allowedOrigins[origin]; !allowed {
				if request.Method == http.MethodOptions {
					http.Error(responseWriter, "origin is not allowed", http.StatusForbidden)
					return
				}
				nextHandler.ServeHTTP(responseWriter, request)
				return
			}

			responseWriter.Header().Set("Access-Control-Allow-Origin", origin)
			responseWriter.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID, X-TracePilot-Refresh, X-Project-ID, X-Environment")
			responseWriter.Header().Set("Access-Control-Allow-Credentials", "true")
			responseWriter.Header().Set("Access-Control-Allow-Methods", allowedMethods)
			responseWriter.Header().Set("Access-Control-Max-Age", CORSMaxAgeSeconds)
			responseWriter.Header().Add("Vary", "Origin")
			if request.Method == http.MethodOptions {
				if !AllowsCORSMethod(allowedMethods, request.Header.Get("Access-Control-Request-Method")) {
					http.Error(responseWriter, "method is not allowed", http.StatusMethodNotAllowed)
					return
				}
				responseWriter.WriteHeader(http.StatusNoContent)
				return
			}
			nextHandler.ServeHTTP(responseWriter, request)
		})
	}
}

// AllowsCORSMethod tests whether a requested method is permitted by the CORS allow-methods string.
func AllowsCORSMethod(methods, wanted string) bool {
	for _, method := range strings.Split(methods, ",") {
		if strings.TrimSpace(method) == wanted {
			return true
		}
	}
	return false
}

// PanicRecovery catches panics, logs the error and returns a 500.
func PanicRecovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(nextHandler http.Handler) http.Handler {
		return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			defer func() {
				if recoveredValue := recover(); recoveredValue != nil {
					if logger != nil {
						logger.Error("http panic", "request_id", request.Header.Get(RequestIDHeader), "error", recoveredValue)
					}
					render.WriteAPIError(responseWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
				}
			}()
			nextHandler.ServeHTTP(responseWriter, request)
		})
	}
}

// NewRequestID generates a 16-byte random hex string.
func NewRequestID() string {
	var randomBytes [16]byte
	if _, randomError := rand.Read(randomBytes[:]); randomError != nil {
		timestamp := time.Now().UTC().Format(time.RFC3339Nano)
		return hex.EncodeToString([]byte(timestamp))
	}
	return hex.EncodeToString(randomBytes[:])
}
