package localauth

import (
	"sync"
	"time"
)

// Failure delays grow per account instead of locking it, so an attacker cannot lock an
// administrator out by guessing. After freeFailures failures, each further attempt
// waits firstDelay, doubling up to maxDelay. NIST SP 800-63B limits consecutive
// failures to 100; with these delays, an account allows fewer than 100 attempts a day.
const (
	freeFailures  = 5
	firstDelay    = 30 * time.Second
	maxDelay      = 15 * time.Minute
	throttleReset = 24 * time.Hour
	// maxThrottled bounds memory. Global and per-client limits slow how fast an attacker
	// can fill it; the oldest idle entries are evicted first.
	maxThrottled = 10000
)

type throttleEntry struct {
	failures int
	last     time.Time
}

// Throttle tracks failed sign-ins per normalized username. Unknown usernames are
// tracked the same way, so the delay does not reveal which accounts exist. The state
// is process memory; the console runs one replica.
type Throttle struct {
	mu      sync.Mutex
	entries map[string]throttleEntry
	now     func() time.Time
}

func NewThrottle() *Throttle {
	return &Throttle{entries: map[string]throttleEntry{}, now: time.Now}
}

func delay(failures int) time.Duration {
	if failures < freeFailures {
		return 0
	}
	wait := firstDelay
	for range failures - freeFailures {
		wait *= 2
		if wait >= maxDelay {
			return maxDelay
		}
	}
	return wait
}

// Allow reports whether key may attempt a sign-in now, or how long it must wait.
func (t *Throttle) Allow(key string) (time.Duration, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry, found := t.entries[key]
	if !found {
		return 0, true
	}
	now := t.now()
	if now.Sub(entry.last) >= throttleReset {
		delete(t.entries, key)
		return 0, true
	}
	if wait := entry.last.Add(delay(entry.failures)).Sub(now); wait > 0 {
		return wait, false
	}
	return 0, true
}

// Failure records a failed attempt for key.
func (t *Throttle) Failure(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	entry, found := t.entries[key]
	if !found && len(t.entries) >= maxThrottled {
		t.evict(now)
	}
	entry.failures++
	entry.last = now
	t.entries[key] = entry
}

// Success clears the failures of key.
func (t *Throttle) Success(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, key)
}

// evict removes expired entries, then the entry with the oldest failure. The caller
// holds t.mu.
func (t *Throttle) evict(now time.Time) {
	oldest, oldestAt := "", now
	for key, entry := range t.entries {
		if now.Sub(entry.last) >= throttleReset {
			delete(t.entries, key)
			continue
		}
		if entry.last.Before(oldestAt) {
			oldest, oldestAt = key, entry.last
		}
	}
	if len(t.entries) >= maxThrottled && oldest != "" {
		delete(t.entries, oldest)
	}
}
