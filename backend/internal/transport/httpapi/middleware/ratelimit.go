package middleware

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/vanquan3008/tracepilot/internal/transport/httpapi/render"
)

const maxRateLimitClients = 4096

type rateLimitBucket struct {
	startedAt time.Time
	count     int
}

// RequestRateLimiter is a bounded fixed-window limiter.
type RequestRateLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	clients map[string]rateLimitBucket
}

// NewRequestRateLimiter creates a new rate limiter with the given window duration.
func NewRequestRateLimiter(window time.Duration) *RequestRateLimiter {
	if window <= 0 {
		window = time.Minute
	}
	return &RequestRateLimiter{window: window, clients: make(map[string]rateLimitBucket)}
}

// Allow checks if the request for the key is within the limit.
func (limiter *RequestRateLimiter) Allow(key string, limit int, now time.Time) (bool, time.Duration) {
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

func (limiter *RequestRateLimiter) evictOldest(now time.Time) {
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

// ClientKeys returns a copy of the active rate limiter keys for inspection.
func (limiter *RequestRateLimiter) ClientKeys() []string {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	keys := make([]string, 0, len(limiter.clients))
	for key := range limiter.clients {
		keys = append(keys, key)
	}
	return keys
}

// RetryAfterSeconds calculates the Retry-After header string.
func RetryAfterSeconds(wait time.Duration) string {
	seconds := int(math.Ceil(wait.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	return strconv.Itoa(seconds)
}

// RateLimit creates an HTTP middleware that limits request rates according to key and limit resolution.
func RateLimit(limiter *RequestRateLimiter, keyResolver func(*http.Request) (string, int)) func(http.Handler) http.Handler {
	return func(nextHandler http.Handler) http.Handler {
		return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			if keyResolver == nil {
				nextHandler.ServeHTTP(responseWriter, request)
				return
			}
			key, limit := keyResolver(request)
			if limit <= 0 {
				nextHandler.ServeHTTP(responseWriter, request)
				return
			}
			allowed, retryAfter := limiter.Allow(key, limit, time.Now())
			if !allowed {
				responseWriter.Header().Set("Retry-After", RetryAfterSeconds(retryAfter))
				render.WriteAPIError(responseWriter, http.StatusTooManyRequests, "RATE_LIMITED", "request rate limit exceeded")
				return
			}
			nextHandler.ServeHTTP(responseWriter, request)
		})
	}
}
