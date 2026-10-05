// Package revocation keeps the gateway's in-memory denylist of revoked session ids
// (ADR-0003 addendum) and the Kafka consumer that feeds it from `identity.events`.
// The gateway still verifies every JWT locally; this only answers "was the session
// this token belongs to revoked?" without a call to identity.
package revocation

import (
	"sync"
	"time"
)

// Denylist is a concurrency-safe set of revoked session ids, each kept until the
// expiry of the longest-lived token that can carry it. An entry past its expiry is
// treated as absent (and swept by Prune) — the token is expired anyway.
type Denylist struct {
	mu      sync.RWMutex
	entries map[string]time.Time // session id -> expires_at
	now     func() time.Time
}

// NewDenylist builds an empty denylist.
func NewDenylist() *Denylist {
	return &Denylist{entries: map[string]time.Time{}, now: time.Now}
}

// Add records a revoked session until expiresAt. Re-adding (a duplicate or replayed
// event) is idempotent: the later expiry wins. An already-expired entry is ignored.
func (d *Denylist) Add(sessionID string, expiresAt time.Time) {
	if sessionID == "" || !expiresAt.After(d.now()) {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if cur, ok := d.entries[sessionID]; !ok || expiresAt.After(cur) {
		d.entries[sessionID] = expiresAt
	}
}

// Revoked reports whether the session is currently denylisted.
func (d *Denylist) Revoked(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	d.mu.RLock()
	exp, ok := d.entries[sessionID]
	d.mu.RUnlock()
	return ok && exp.After(d.now())
}

// Prune drops entries whose expiry has passed and returns how many were removed.
func (d *Denylist) Prune() int {
	now := d.now()
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for id, exp := range d.entries {
		if !exp.After(now) {
			delete(d.entries, id)
			n++
		}
	}
	return n
}

// Len is the number of held entries (expired-but-unpruned ones included).
func (d *Denylist) Len() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.entries)
}
