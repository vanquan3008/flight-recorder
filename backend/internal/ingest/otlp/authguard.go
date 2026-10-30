package otlp

import (
	"crypto/sha256"
	"errors"
	"sync"
	"time"
)

// badKeyTTL is how long a rejected key is remembered. It is short so a key that
// is created right after a failed attempt is usable almost at once.
const badKeyTTL = 5 * time.Second

// maxGuardEntries bounds each table so a flood of distinct clients or keys
// cannot grow memory.
const maxGuardEntries = 4096

// ErrThrottled is returned while a client is over its failed-authentication budget.
var ErrThrottled = errors.New("too many failed authentication attempts")

type throttledError struct{ retryAfter time.Duration }

func (*throttledError) Error() string        { return ErrThrottled.Error() }
func (*throttledError) Is(target error) bool { return target == ErrThrottled }

type clientFailures struct {
	windowStart time.Time
	count       int
}

// authGuard keeps rejected credentials from becoming database load. Every OTLP
// request is authenticated with a database lookup, so without it a client
// sending junk keys occupies the pool that ingestion shares. It does two
// things:
//   - remembers recently rejected keys (by digest, never the key itself), so a
//     retried bad key costs no lookup;
//   - counts failures per client and, past the limit, answers without a lookup
//     until the window ends. Only failures count, so a client with a valid key
//     is unaffected until it also fails.
//
// Accepted keys are never cached: a revoked key must stop working at once.
// A nil guard allows everything.
type authGuard struct {
	mu      sync.Mutex
	now     func() time.Time
	limit   int
	window  time.Duration
	clients map[string]clientFailures
	badKeys map[[sha256.Size]byte]time.Time
}

func newAuthGuard(limit int, window time.Duration) *authGuard {
	if limit <= 0 || window <= 0 {
		return nil
	}
	return &authGuard{
		now: time.Now, limit: limit, window: window,
		clients: make(map[string]clientFailures), badKeys: make(map[[sha256.Size]byte]time.Time),
	}
}

// throttled reports whether the client is over budget and for how much longer.
func (guard *authGuard) throttled(client string) (time.Duration, bool) {
	if guard == nil {
		return 0, false
	}
	guard.mu.Lock()
	defer guard.mu.Unlock()
	entry, found := guard.clients[client]
	if !found {
		return 0, false
	}
	elapsed := guard.now().Sub(entry.windowStart)
	if elapsed >= guard.window || entry.count < guard.limit {
		return 0, false
	}
	return guard.window - elapsed, true
}

// knownBad reports whether the key was rejected within badKeyTTL.
func (guard *authGuard) knownBad(rawKey string) bool {
	if guard == nil {
		return false
	}
	digest := sha256.Sum256([]byte(rawKey))
	guard.mu.Lock()
	defer guard.mu.Unlock()
	expires, found := guard.badKeys[digest]
	return found && guard.now().Before(expires)
}

// failed records one failed attempt. rememberKey is set when the key itself is
// unknown; a valid key that merely lacks the ingest scope is not remembered.
func (guard *authGuard) failed(client, rawKey string, rememberKey bool) {
	if guard == nil {
		return
	}
	now := guard.now()
	guard.mu.Lock()
	defer guard.mu.Unlock()
	entry, found := guard.clients[client]
	if !found || now.Sub(entry.windowStart) >= guard.window {
		if !found && len(guard.clients) >= maxGuardEntries {
			guard.makeRoomForClient(now)
		}
		entry = clientFailures{windowStart: now}
	}
	entry.count++
	guard.clients[client] = entry
	if rememberKey {
		if len(guard.badKeys) >= maxGuardEntries {
			guard.makeRoomForKey(now)
		}
		guard.badKeys[sha256.Sum256([]byte(rawKey))] = now.Add(badKeyTTL)
	}
}

// makeRoomForClient drops expired windows, then one arbitrary entry if the
// table is still full.
func (guard *authGuard) makeRoomForClient(now time.Time) {
	for client, entry := range guard.clients {
		if now.Sub(entry.windowStart) >= guard.window {
			delete(guard.clients, client)
		}
	}
	for client := range guard.clients {
		if len(guard.clients) < maxGuardEntries {
			return
		}
		delete(guard.clients, client)
	}
}

func (guard *authGuard) makeRoomForKey(now time.Time) {
	for digest, expires := range guard.badKeys {
		if !now.Before(expires) {
			delete(guard.badKeys, digest)
		}
	}
	for digest := range guard.badKeys {
		if len(guard.badKeys) < maxGuardEntries {
			return
		}
		delete(guard.badKeys, digest)
	}
}
