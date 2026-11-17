package httpapi

import (
	"net/http"
	"strings"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/middleware"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

// This file binds the generic middleware package to the server's configuration.

const (
	corsMaxAgeSeconds = middleware.CORSMaxAgeSeconds
	requestIDHeader   = render.RequestIDHeader
)

var (
	metricMethod      = middleware.MetricMethod
	allowsCORSMethod  = middleware.AllowsCORSMethod
	retryAfterSeconds = middleware.RetryAfterSeconds
)

func (server *Server) withAccessLog(nextHandler http.Handler) http.Handler {
	return middleware.AccessLog(server.logger)(nextHandler)
}

func (server *Server) withMetrics(nextHandler http.Handler) http.Handler {
	return middleware.Metrics(server.metrics)(nextHandler)
}

func (server *Server) withBodyLimit(nextHandler http.Handler) http.Handler {
	return middleware.BodyLimit(server.maxBodyBytes)(nextHandler)
}

func (server *Server) withRequestID(nextHandler http.Handler) http.Handler {
	return middleware.RequestID()(nextHandler)
}

func (server *Server) withSecurityHeaders(nextHandler http.Handler) http.Handler {
	return middleware.SecurityHeaders(server.certificateFile != "")(nextHandler)
}

func (server *Server) withCORS(nextHandler http.Handler) http.Handler {
	return middleware.CORS(server.allowedOrigins, server.corsAllowedMethods)(nextHandler)
}

func (server *Server) withPanicRecovery(nextHandler http.Handler) http.Handler {
	return middleware.PanicRecovery(server.logger)(nextHandler)
}

func (server *Server) withRateLimit(nextHandler http.Handler) http.Handler {
	return middleware.RateLimit(server.rateLimiter, server.rateLimitKey)(nextHandler)
}

// rateLimitKey assigns an API request to its limit class and keys it on the
// client address; requests outside /api/v1 have no limit.
func (server *Server) rateLimitKey(request *http.Request) (string, int) {
	if !strings.HasPrefix(request.URL.Path, "/api/v1/") {
		return "", 0
	}
	class, limit := "query", server.rateLimitRequests
	switch {
	case strings.HasSuffix(request.URL.Path, "/replays"):
		class, limit = "replay", server.rateLimitReplayRequests
	case strings.HasPrefix(request.URL.Path, "/api/v1/auth/") || strings.HasSuffix(request.URL.Path, "/ingest-tokens"):
		class, limit = "auth", server.rateLimitAuthRequests
	}
	return class + ":" + server.clientAddress(request), limit
}

// clientAddress is the address that rate limits are keyed on; see
// config.ClientAddress for how forwarded headers are trusted.
func (server *Server) clientAddress(request *http.Request) string {
	return config.ClientAddress(request.RemoteAddr, request.Header.Values("X-Forwarded-For"), server.trustedProxies)
}
