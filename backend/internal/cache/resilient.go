package cache

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// CircuitBreakerState represents the operational state of the Redis circuit breaker.
type CircuitBreakerState int32

const (
	// StateClosed: Redis is healthy and receiving all cache traffic.
	StateClosed CircuitBreakerState = 0
	// StateOpen: Redis is unavailable; all traffic immediately fails over to in-memory fallback.
	StateOpen CircuitBreakerState = 1
	// StateHalfOpen: Cooldown has elapsed; testing Redis with a probe request to verify recovery.
	StateHalfOpen CircuitBreakerState = 2
)

func (s CircuitBreakerState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

const (
	defaultMaxFailures = 3
	defaultCooldown    = 10 * time.Second
)

// ResilientCache wraps a primary Redis cache with an in-memory fallback protected by a circuit breaker.
// It guarantees zero downtime by degrading gracefully to the local in-memory cache whenever Redis fails,
// and automatically resumes distributed caching once Redis recovers.
type ResilientCache struct {
	redis   *RedisCache
	memory  *MemoryCache
	logger  *slog.Logger
	metrics Metrics

	mu                  sync.Mutex
	state               atomic.Int32
	consecutiveFailures atomic.Int32
	lastStateChange     time.Time
	maxFailures         int32
	cooldown            time.Duration
	now                 func() time.Time
}

// NewResilientCache constructs a ResilientCache with primary Redis, fallback Memory, and circuit breaker.
func NewResilientCache(
	redis *RedisCache,
	memory *MemoryCache,
	logger *slog.Logger,
	metrics Metrics,
	redisHealthy bool,
) *ResilientCache {
	if logger == nil {
		logger = slog.Default()
	}
	if memory == nil {
		memory = NewMemoryCache(60*time.Second, 4096)
	}

	rc := &ResilientCache{
		redis:       redis,
		memory:      memory,
		logger:      logger,
		metrics:     metrics,
		maxFailures: defaultMaxFailures,
		cooldown:    defaultCooldown,
		now:         time.Now,
	}

	if redisHealthy && redis != nil {
		rc.state.Store(int32(StateClosed))
		rc.lastStateChange = rc.currentTime()
	} else {
		rc.state.Store(int32(StateOpen))
		rc.lastStateChange = rc.currentTime()
		if metrics != nil {
			metrics.RecordCircuitBreakerState("open")
		}
	}

	return rc
}

func (r *ResilientCache) currentTime() time.Time {
	if r != nil && r.now != nil {
		return r.now()
	}
	return time.Now()
}

// State returns the current circuit breaker state.
func (r *ResilientCache) State() CircuitBreakerState {
	return CircuitBreakerState(r.state.Load())
}

// Get retrieves the value, preferring Redis when healthy and falling back to memory.
func (r *ResilientCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if r.shouldAttemptRedis() {
		val, found, err := r.redis.Get(ctx, key)
		if err == nil {
			r.onSuccess()
			if found {
				return val, true, nil
			}
			// Cache miss in Redis: check memory fallback in case of recent local writes
			return r.memory.Get(ctx, key)
		}

		r.onFailure(ctx, "read", key, err)
	}

	if r.metrics != nil {
		r.metrics.RecordFallback("get")
	}
	return r.memory.Get(ctx, key)
}

// Set stores the value in both in-memory fallback (warm cache) and Redis when circuit allows.
func (r *ResilientCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	// Always keep memory fallback populated so failover is instantaneous
	_ = r.memory.Set(ctx, key, value, ttl)

	if r.shouldAttemptRedis() {
		err := r.redis.Set(ctx, key, value, ttl)
		if err == nil {
			r.onSuccess()
			return nil
		}
		r.onFailure(ctx, "write", key, err)
		if r.metrics != nil {
			r.metrics.RecordFallback("set")
		}
	}

	return nil
}

// Delete evicts the key from both memory fallback and Redis.
func (r *ResilientCache) Delete(ctx context.Context, key string) error {
	_ = r.memory.Delete(ctx, key)

	if r.shouldAttemptRedis() {
		err := r.redis.Delete(ctx, key)
		if err == nil {
			r.onSuccess()
			return nil
		}
		r.onFailure(ctx, "delete", key, err)
		if r.metrics != nil {
			r.metrics.RecordFallback("delete")
		}
	}

	return nil
}

// Close gracefully closes Redis and clears memory cache.
func (r *ResilientCache) Close() error {
	var redisErr error
	if r.redis != nil {
		redisErr = r.redis.Close()
	}
	memErr := r.memory.Close()
	if redisErr != nil {
		return redisErr
	}
	return memErr
}

// shouldAttemptRedis checks the circuit breaker state to decide whether to call Redis.
func (r *ResilientCache) shouldAttemptRedis() bool {
	if r.redis == nil {
		return false
	}

	currentState := CircuitBreakerState(r.state.Load())
	if currentState == StateClosed || currentState == StateHalfOpen {
		return true
	}

	// State is Open: check if cooldown has elapsed to transition to HalfOpen
	now := r.currentTime()
	r.mu.Lock()
	defer r.mu.Unlock()

	// Double check state after lock acquisition
	if CircuitBreakerState(r.state.Load()) == StateOpen {
		if now.Sub(r.lastStateChange) >= r.cooldown {
			r.state.Store(int32(StateHalfOpen))
			r.lastStateChange = now
			r.logger.Info("redis circuit breaker entered HALF-OPEN state; probing connection")
			if r.metrics != nil {
				r.metrics.RecordCircuitBreakerState("half-open")
			}
			return true
		}
	}

	return false
}

// onSuccess records a successful Redis call, recovering the circuit if it was open or half-open.
func (r *ResilientCache) onSuccess() {
	r.consecutiveFailures.Store(0)

	if CircuitBreakerState(r.state.Load()) != StateClosed {
		r.mu.Lock()
		defer r.mu.Unlock()

		if CircuitBreakerState(r.state.Load()) != StateClosed {
			r.state.Store(int32(StateClosed))
			r.lastStateChange = r.currentTime()
			r.logger.Info("redis cache recovered; circuit breaker CLOSED, distributed caching resumed")
			if r.metrics != nil {
				r.metrics.RecordCircuitBreakerState("closed")
			}
		}
	}
}

// onFailure records a Redis error and trips the circuit breaker if failure threshold is reached.
func (r *ResilientCache) onFailure(ctx context.Context, op, key string, err error) {
	failures := r.consecutiveFailures.Add(1)

	r.mu.Lock()
	defer r.mu.Unlock()

	currentState := CircuitBreakerState(r.state.Load())

	if currentState == StateHalfOpen {
		// Probe failed in HalfOpen: trip immediately back to Open
		r.state.Store(int32(StateOpen))
		r.lastStateChange = r.currentTime()
		r.logger.WarnContext(ctx, "redis probe failed in half-open state; circuit breaker re-OPENED",
			slog.String("operation", op),
			slog.String("key", key),
			slog.Any("error", err),
		)
		if r.metrics != nil {
			r.metrics.RecordCircuitBreakerState("open")
		}
		return
	}

	if currentState == StateClosed && failures >= r.maxFailures {
		// Threshold exceeded: open circuit
		r.state.Store(int32(StateOpen))
		r.lastStateChange = r.currentTime()
		r.logger.WarnContext(ctx, "redis cache consecutive failures threshold exceeded; circuit breaker OPENED, falling back to in-memory cache",
			slog.Int("consecutive_failures", int(failures)),
			slog.String("operation", op),
			slog.String("key", key),
			slog.Any("error", err),
		)
		if r.metrics != nil {
			r.metrics.RecordCircuitBreakerState("open")
		}
	} else if currentState == StateClosed {
		r.logger.DebugContext(ctx, "redis cache operation encountered transient error",
			slog.Int("consecutive_failures", int(failures)),
			slog.String("operation", op),
			slog.String("key", key),
			slog.Any("error", err),
		)
	}
}
