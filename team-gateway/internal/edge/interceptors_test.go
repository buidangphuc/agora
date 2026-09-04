package edge

import (
	"strconv"
	"testing"
	"time"
)

// TestRateLimiterEvictsIdleKeys reproduces the unbounded-map finding (SA-Med):
// after inserting many distinct keys and idling past the TTL, the map must be
// bounded (idle keys evicted), not M-and-growing.
func TestRateLimiterEvictsIdleKeys(t *testing.T) {
	rl := newRateLimiter(100, 100)
	rl.ttl = time.Minute

	now := time.Unix(0, 0)
	rl.now = func() time.Time { return now }

	const m = 500
	for i := 0; i < m; i++ {
		rl.allow("user:" + strconv.Itoa(i))
	}
	if got := rl.size(); got != m {
		t.Fatalf("expected %d live keys before TTL, got %d", m, got)
	}

	// Idle every key past the TTL, then a single call triggers the sweep.
	now = now.Add(2 * time.Minute)
	rl.allow("user:active")

	if got := rl.size(); got > 1 {
		t.Fatalf("expected idle keys evicted (bounded map), got %d entries", got)
	}
}

// TestRateLimiterKeepsActiveKey guards against a regression: a key that keeps
// being used within the TTL must survive the sweep that evicts idle keys.
func TestRateLimiterKeepsActiveKey(t *testing.T) {
	rl := newRateLimiter(100, 100)
	rl.ttl = time.Minute

	now := time.Unix(0, 0)
	rl.now = func() time.Time { return now }

	// Insert throwaway keys once, then keep touching the active key at intervals
	// shorter than the TTL so it is never idle when the sweep finally runs.
	for i := 0; i < 50; i++ {
		rl.allow("user:throwaway-" + strconv.Itoa(i))
	}
	rl.allow("user:active")

	now = now.Add(30 * time.Second) // < ttl: sweep gated, active refreshed
	rl.allow("user:active")

	now = now.Add(40 * time.Second) // now 70s past insert: sweep runs
	rl.allow("user:active")         // active idle only 40s (< ttl) -> survives

	if got := rl.size(); got != 1 {
		t.Fatalf("expected only the active key to survive the sweep, got %d entries", got)
	}

	// No regression: a key's bucket still enforces the configured burst limit.
	limited := newRateLimiter(1, 3) // 1 rps, burst 3
	allowed := 0
	for i := 0; i < 10; i++ {
		if limited.allow("user:bob") {
			allowed++
		}
	}
	if allowed != 3 {
		t.Fatalf("burst 3 should allow exactly 3 immediate calls, got %d", allowed)
	}
}
