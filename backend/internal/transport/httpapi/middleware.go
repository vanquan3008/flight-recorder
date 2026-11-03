package httpapi

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/vanquan3008/tracepilot/internal/config"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/middleware"
	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

const (
	corsMaxAgeSeconds   = middleware.CORSMaxAgeSeconds
	requestIDHeader     = render.RequestIDHeader
	maxRateLimitClients = 4096
)

var (
	newRequestID      = middleware.NewRequestID
	metricMethod      = middleware.MetricMethod
	allowsCORSMethod  = middleware.AllowsCORSMethod
	retryAfterSeconds = middleware.RetryAfterSeconds
)

type rateLimitBucket struct {
	startedAt time.Time
	count     int
}

type requestRateLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	clients map[string]rateLimitBucket
}

func newRequestRateLimiter(window time.Duration) *requestRateLimiter {
	if window <= 0 {
		window = time.Minute
	}
	return &requestRateLimiter{window: window, clients: make(map[string]rateLimitBucket)}
}

func (limiter *requestRateLimiter) allow(key string, limit int, now time.Time) (bool, time.Duration) {
	if limit <= 0 {
		return false, limiter.window
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if bucket, exists := limiter.clients[key]; exists && now.Sub(bucket.startedAt) < limiter.window {
		if bucket.count >= limit {
			return false, limiter.window - now.Sub(bucket.startedAt)
		}
		bucket.count++
		limiter.clients[key] = bucket
		return true, 0
	}
	if len(limiter.clients) >= maxRateLimitClients {
		limiter.evictOldest(now)
	}
	limiter.clients[key] = rateLimitBucket{startedAt: now, count: 1}
	return true, 0
}

func (limiter *requestRateLimiter) Allow(key string, limit int, now time.Time) (bool, time.Duration) {
	return limiter.allow(key, limit, now)
}

func (limiter *requestRateLimiter) ClientKeys() []string {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	keys := make([]string, 0, len(limiter.clients))
	for k := range limiter.clients {
		keys = append(keys, k)
	}
	return keys
}

func (limiter *requestRateLimiter) evictOldest(now time.Time) {
	for key, bucket := range limiter.clients {
		if now.Sub(bucket.startedAt) >= limiter.window {
			delete(limiter.clients, key)
		}
	}
	if len(limiter.clients) >= maxRateLimitClients {
		threshold := now.Add(-limiter.window / 2)
		evicted := 0
		for key, bucket := range limiter.clients {
			if bucket.startedAt.Before(threshold) {
				delete(limiter.clients, key)
				evicted++
				if evicted >= maxRateLimitClients/4 {
					break
				}
			}
		}
		if len(limiter.clients) >= maxRateLimitClients {
			for key := range limiter.clients {
				delete(limiter.clients, key)
				evicted++
				if evicted >= maxRateLimitClients/4 {
					break
				}
			}
		}
	}
}

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
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		limit, class := server.rateLimitFor(request)
		if limit <= 0 {
			nextHandler.ServeHTTP(responseWriter, request)
			return
		}
		key := class + ":" + server.clientAddress(request)
		allowed, retryAfter := server.rateLimiter.Allow(key, limit, time.Now())
		if !allowed {
			responseWriter.Header().Set("Retry-After", retryAfterSeconds(retryAfter))
			writeAPIError(responseWriter, http.StatusTooManyRequests, "RATE_LIMITED", "request rate limit exceeded")
			return
		}
		nextHandler.ServeHTTP(responseWriter, request)
	})
}

func (server *Server) rateLimitFor(request *http.Request) (int, string) {
	if !strings.HasPrefix(request.URL.Path, "/api/v1/") {
		return 0, ""
	}
	if strings.HasSuffix(request.URL.Path, "/replays") {
		return server.rateLimitReplayRequests, "replay"
	}
	if strings.HasPrefix(request.URL.Path, "/api/v1/auth/") || strings.HasSuffix(request.URL.Path, "/ingest-tokens") {
		return server.rateLimitAuthRequests, "auth"
	}
	return server.rateLimitRequests, "query"
}

// clientAddress is the address that rate limits are keyed on; see
// config.ClientAddress for how forwarded headers are trusted.
func (server *Server) clientAddress(request *http.Request) string {
	return config.ClientAddress(request.RemoteAddr, request.Header.Values("X-Forwarded-For"), server.trustedProxies)
}
