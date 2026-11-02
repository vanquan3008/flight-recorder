package httpapi

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxRateLimitClients = 4096

type rateLimitBucket struct {
	startedAt time.Time
	count     int
}

// requestRateLimiter is deliberately a bounded fixed-window limiter. It is
// process-local (the v1 deployment has one API replica) and evicts the oldest
// window when a client flood would otherwise grow the map without bound.
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

func (limiter *requestRateLimiter) evictOldest(now time.Time) {
	oldestKey := ""
	oldestAt := now
	for key, bucket := range limiter.clients {
		if now.Sub(bucket.startedAt) >= limiter.window {
			delete(limiter.clients, key)
			continue
		}
		if oldestKey == "" || bucket.startedAt.Before(oldestAt) {
			oldestKey, oldestAt = key, bucket.startedAt
		}
	}
	if oldestKey != "" {
		delete(limiter.clients, oldestKey)
	}
}

func (server *Server) withRateLimit(nextHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		limit, class := server.rateLimitFor(request)
		if limit <= 0 {
			nextHandler.ServeHTTP(responseWriter, request)
			return
		}
		key := class + ":" + server.clientAddress(request)
		allowed, retryAfter := server.rateLimiter.allow(key, limit, time.Now())
		if !allowed {
			responseWriter.Header().Set("Retry-After", retryAfterSeconds(retryAfter))
			writeAPIError(responseWriter, http.StatusTooManyRequests, "RATE_LIMITED", "request rate limit exceeded")
			return
		}
		nextHandler.ServeHTTP(responseWriter, request)
	})
}

func retryAfterSeconds(wait time.Duration) string {
	seconds := int(math.Ceil(wait.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	return strconv.Itoa(seconds)
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
