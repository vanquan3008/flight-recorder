package cache

import (
	"sync"
	"sync/atomic"
)

// Metrics defines the monitoring contract for cache operations and health events.
type Metrics interface {
	RecordHit(driver string)
	RecordMiss(driver string)
	RecordSet(driver string)
	RecordDelete(driver string)
	RecordEviction(driver string, reason string)
	RecordError(driver string, op string)
	RecordFallback(op string)
	RecordCircuitBreakerState(state string)
}

// StatsSnapshot captures an immutable point-in-time view of cache statistics.
type StatsSnapshot struct {
	HitsTotal        int64            `json:"hits_total"`
	MissesTotal      int64            `json:"misses_total"`
	SetsTotal        int64            `json:"sets_total"`
	DeletesTotal     int64            `json:"deletes_total"`
	EvictionsTotal   int64            `json:"evictions_total"`
	ErrorsTotal      int64            `json:"errors_total"`
	FallbacksTotal   int64            `json:"fallbacks_total"`
	HitsByDriver     map[string]int64 `json:"hits_by_driver"`
	MissesByDriver   map[string]int64 `json:"misses_by_driver"`
	SetsByDriver     map[string]int64 `json:"sets_by_driver"`
	DeletesByDriver  map[string]int64 `json:"deletes_by_driver"`
	EvictionsDetail  map[string]int64 `json:"evictions_detail"`
	ErrorsDetail     map[string]int64 `json:"errors_detail"`
	FallbacksDetail  map[string]int64 `json:"fallbacks_detail"`
	LastCircuitState string           `json:"last_circuit_state"`
}

// StatsCollector provides a thread-safe, lock-free implementation of Metrics using atomic counters.
type StatsCollector struct {
	mu               sync.RWMutex
	hitsByDriver     map[string]*atomic.Int64
	missesByDriver   map[string]*atomic.Int64
	setsByDriver     map[string]*atomic.Int64
	deletesByDriver  map[string]*atomic.Int64
	evictions        map[string]*atomic.Int64
	errors           map[string]*atomic.Int64
	fallbacks        map[string]*atomic.Int64
	lastCircuitState atomic.Pointer[string]

	totalHits      atomic.Int64
	totalMisses    atomic.Int64
	totalSets      atomic.Int64
	totalDeletes   atomic.Int64
	totalEvictions atomic.Int64
	totalErrors    atomic.Int64
	totalFallbacks atomic.Int64
}

// NewStatsCollector constructs an initialized StatsCollector with pre-allocated common drivers.
func NewStatsCollector() *StatsCollector {
	sc := &StatsCollector{
		hitsByDriver: map[string]*atomic.Int64{
			"memory": {},
			"redis":  {},
		},
		missesByDriver: map[string]*atomic.Int64{
			"memory": {},
			"redis":  {},
		},
		setsByDriver: map[string]*atomic.Int64{
			"memory": {},
			"redis":  {},
		},
		deletesByDriver: map[string]*atomic.Int64{
			"memory": {},
			"redis":  {},
		},
		evictions: make(map[string]*atomic.Int64),
		errors:    make(map[string]*atomic.Int64),
		fallbacks: make(map[string]*atomic.Int64),
	}
	initialState := "closed"
	sc.lastCircuitState.Store(&initialState)
	return sc
}

func (s *StatsCollector) getCounter(m map[string]*atomic.Int64, key string) *atomic.Int64 {
	s.mu.RLock()
	counter, exists := m[key]
	s.mu.RUnlock()
	if exists {
		return counter
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if counter, exists = m[key]; exists {
		return counter
	}
	counter = &atomic.Int64{}
	m[key] = counter
	return counter
}

// RecordHit increments the hit counter for the specified driver.
func (s *StatsCollector) RecordHit(driver string) {
	s.totalHits.Add(1)
	s.getCounter(s.hitsByDriver, driver).Add(1)
}

// RecordMiss increments the miss counter for the specified driver.
func (s *StatsCollector) RecordMiss(driver string) {
	s.totalMisses.Add(1)
	s.getCounter(s.missesByDriver, driver).Add(1)
}

// RecordSet increments the set counter for the specified driver.
func (s *StatsCollector) RecordSet(driver string) {
	s.totalSets.Add(1)
	s.getCounter(s.setsByDriver, driver).Add(1)
}

// RecordDelete increments the delete counter for the specified driver.
func (s *StatsCollector) RecordDelete(driver string) {
	s.totalDeletes.Add(1)
	s.getCounter(s.deletesByDriver, driver).Add(1)
}

// RecordEviction increments the eviction counter for the given reason.
func (s *StatsCollector) RecordEviction(driver string, reason string) {
	s.totalEvictions.Add(1)
	s.getCounter(s.evictions, driver+":"+reason).Add(1)
}

// RecordError increments the error counter for the given operation.
func (s *StatsCollector) RecordError(driver string, op string) {
	s.totalErrors.Add(1)
	s.getCounter(s.errors, driver+":"+op).Add(1)
}

// RecordFallback increments the fallback counter for the given operation.
func (s *StatsCollector) RecordFallback(op string) {
	s.totalFallbacks.Add(1)
	s.getCounter(s.fallbacks, op).Add(1)
}

// RecordCircuitBreakerState stores the most recent circuit breaker state.
func (s *StatsCollector) RecordCircuitBreakerState(state string) {
	stateCopy := state
	s.lastCircuitState.Store(&stateCopy)
}

// Snapshot returns an immutable point-in-time snapshot of all metrics.
func (s *StatsCollector) Snapshot() StatsSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	copyMap := func(m map[string]*atomic.Int64) map[string]int64 {
		res := make(map[string]int64, len(m))
		for k, v := range m {
			res[k] = v.Load()
		}
		return res
	}

	var state string
	if ptr := s.lastCircuitState.Load(); ptr != nil {
		state = *ptr
	}

	return StatsSnapshot{
		HitsTotal:        s.totalHits.Load(),
		MissesTotal:      s.totalMisses.Load(),
		SetsTotal:        s.totalSets.Load(),
		DeletesTotal:     s.totalDeletes.Load(),
		EvictionsTotal:   s.totalEvictions.Load(),
		ErrorsTotal:      s.totalErrors.Load(),
		FallbacksTotal:   s.totalFallbacks.Load(),
		HitsByDriver:     copyMap(s.hitsByDriver),
		MissesByDriver:   copyMap(s.missesByDriver),
		SetsByDriver:     copyMap(s.setsByDriver),
		DeletesByDriver:  copyMap(s.deletesByDriver),
		EvictionsDetail:  copyMap(s.evictions),
		ErrorsDetail:     copyMap(s.errors),
		FallbacksDetail:  copyMap(s.fallbacks),
		LastCircuitState: state,
	}
}
