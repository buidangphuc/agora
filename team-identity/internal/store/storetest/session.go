package storetest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-identity/internal/store"
)

// --- InMemory implementation (tests, no live DB) ---

type InMemorySessionRepository struct {
	mu       sync.RWMutex
	sessions map[string]store.Session
	logins   []store.LoginEvent
	seq      int64
	outbox   []store.OutboxRow
	users    *InMemoryUserRepository // set by AttachSessions

	// FailOutbox makes the outbox write of a revoke fail, exercising the
	// rollback path (the session must stay un-revoked and no row is recorded).
	FailOutbox bool
}

func NewInMemorySessionRepository() *InMemorySessionRepository {
	return &InMemorySessionRepository{sessions: make(map[string]store.Session)}
}

func (r *InMemorySessionRepository) CreateSession(_ context.Context, s store.Session) (store.Session, error) {
	if r.users != nil {
		// Lock order: users then sessions (same as SetStatus).
		r.users.mu.RLock()
		defer r.users.mu.RUnlock()
		if u, ok := r.users.byID[s.UserID]; ok {
			if u.Status == store.StatusSuspended || u.Status == store.StatusBanned {
				return store.Session{}, store.ErrAccountLocked
			}
			s.UserStatus = u.Status
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if s.CreatedAt.IsZero() {
		// Monotonic-ish ordering for deterministic tests.
		r.seq++
		s.CreatedAt = now.Add(time.Duration(r.seq) * time.Millisecond)
	}
	if s.LastSeen.IsZero() {
		s.LastSeen = s.CreatedAt
	}
	r.sessions[s.ID] = s
	return s, nil
}

func (r *InMemorySessionRepository) ListSessions(_ context.Context, userID string) ([]store.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var items []store.Session
	for _, s := range r.sessions {
		if s.UserID == userID {
			items = append(items, s)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].LastSeen.Equal(items[j].LastSeen) {
			return items[i].LastSeen.After(items[j].LastSeen)
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items, nil
}

func (r *InMemorySessionRepository) RevokeSession(_ context.Context, sessionID, userID string, ev store.EnqueueFn) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[sessionID]
	if !ok || s.UserID != userID {
		return store.ErrSessionNotFound
	}
	s.Revoked = true
	s.RefreshHash, s.PrevRefreshHash = "", ""
	// Mirror the Postgres transaction: build/record the outbox row first and only
	// then commit the revoke, so a failing outbox write leaves the session intact.
	if ev != nil {
		row, err := ev(s)
		if err != nil {
			return fmt.Errorf("build revoke event: %w: %w", store.ErrRotateFault, err)
		}
		if r.FailOutbox {
			return fmt.Errorf("enqueue revoke event: %w", store.ErrRotateFault)
		}
		r.outbox = append(r.outbox, row)
	}
	r.sessions[sessionID] = s
	return nil
}

func (r *InMemorySessionRepository) RotateRefresh(_ context.Context, presentedHash, newHash string, now time.Time, grace, maxAge time.Duration, ip, device string, ev store.EnqueueFn) (store.RotateResult, error) {
	if ev == nil {
		return store.RotateResult{}, errEnqueueRequired
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if presentedHash == "" {
		return store.RotateResult{Outcome: store.RotateStale}, nil
	}
	var s store.Session
	found := false
	for _, c := range r.sessions {
		if !c.Revoked && (c.RefreshHash == presentedHash || c.PrevRefreshHash == presentedHash) {
			s, found = c, true
			break
		}
	}
	if !found {
		return store.RotateResult{Outcome: store.RotateStale}, nil
	}
	revoke := func(outcome store.RotateOutcome) (store.RotateResult, error) {
		s.Revoked = true
		s.RefreshHash, s.PrevRefreshHash = "", ""
		row, err := ev(s)
		if err != nil {
			return store.RotateResult{}, fmt.Errorf("build revoke event: %w: %w", store.ErrRotateFault, err)
		}
		if r.FailOutbox {
			return store.RotateResult{}, fmt.Errorf("enqueue revoke event: %w", store.ErrRotateFault)
		}
		r.outbox = append(r.outbox, row)
		r.sessions[s.ID] = s
		return store.RotateResult{Outcome: outcome, Session: s}, nil
	}
	if now.Sub(s.CreatedAt) > maxAge {
		return revoke(store.RotateLifetimeExceeded)
	}
	if !s.RefreshExpiresAt.IsZero() && !now.Before(s.RefreshExpiresAt) {
		return store.RotateResult{Outcome: store.RotateExpired, Session: s}, nil
	}
	switch {
	case s.RefreshHash == presentedHash:
		for _, c := range r.sessions {
			if c.ID != s.ID && (c.RefreshHash == newHash) && newHash != "" {
				return store.RotateResult{}, fmt.Errorf("%w: %w", errRefreshHashConflict, store.ErrRotateFault)
			}
		}
		s.PrevRefreshHash = s.RefreshHash
		s.RefreshHash = newHash
		s.RotatedAt = now
		s.LastSeen = now
		if ip != "" {
			s.IP = ip
		}
		if device != "" {
			s.Device = device
		}
		s.RefreshExpiresAt = now.Add(store.RefreshTTL)
		r.sessions[s.ID] = s
		return store.RotateResult{Outcome: store.RotateRotated, Session: s}, nil
	case !s.RotatedAt.IsZero() && now.Sub(s.RotatedAt) <= grace:
		return store.RotateResult{Outcome: store.RotateReplayedInGrace, Session: s}, nil
	default:
		return revoke(store.RotateReuse)
	}
}

// OutboxRows returns a copy of the outbox rows recorded by revokes, for assertions.
func (r *InMemorySessionRepository) OutboxRows() []store.OutboxRow {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]store.OutboxRow(nil), r.outbox...)
}

func (r *InMemorySessionRepository) RecordLogin(_ context.Context, e store.LoginEvent) (store.LoginEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.CreatedAt.IsZero() {
		r.seq++
		e.CreatedAt = time.Now().UTC().Add(time.Duration(r.seq) * time.Millisecond)
	}
	r.logins = append(r.logins, e)
	return e, nil
}

func (r *InMemorySessionRepository) ListLoginHistory(_ context.Context, userID string, limit, offset int) ([]store.LoginEvent, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var owned []store.LoginEvent
	for _, e := range r.logins {
		if e.UserID == userID {
			owned = append(owned, e)
		}
	}
	sort.Slice(owned, func(i, j int) bool {
		if !owned[i].CreatedAt.Equal(owned[j].CreatedAt) {
			return owned[i].CreatedAt.After(owned[j].CreatedAt)
		}
		return owned[i].ID > owned[j].ID
	})

	total := int64(len(owned))
	if offset < 0 {
		offset = 0
	}
	if offset >= len(owned) {
		return nil, total, nil
	}
	end := offset + limit
	if limit <= 0 || end > len(owned) {
		end = len(owned)
	}
	return owned[offset:end], total, nil
}

// PurgeSessions: the in-memory model has no revoked_at, so revoked rows age from LastSeen.
func (r *InMemorySessionRepository) PurgeSessions(_ context.Context, now time.Time, retention time.Duration, limit int) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cutoff := now.Add(-retention)
	absCutoff := now.Add(-retention - sessionAbsoluteMaxAge)
	ids := make([]string, 0)
	for id, s := range r.sessions {
		var dead bool
		if s.Revoked {
			dead = s.LastSeen.Before(cutoff)
		} else {
			dead = (!s.RefreshExpiresAt.IsZero() && s.RefreshExpiresAt.Before(cutoff)) || s.CreatedAt.Before(absCutoff)
		}
		if dead {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if limit >= 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	for _, id := range ids {
		delete(r.sessions, id)
	}
	return len(ids), nil
}

func (r *InMemorySessionRepository) PurgeLoginHistory(_ context.Context, before time.Time, limit int) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := make([]store.LoginEvent, 0, len(r.logins))
	n := 0
	for _, e := range r.logins {
		if e.CreatedAt.Before(before) && (limit < 0 || n < limit) {
			n++
			continue
		}
		kept = append(kept, e)
	}
	r.logins = kept
	return n, nil
}

var (
	errRefreshHashConflict = errors.New("refresh hash conflict")
	errEnqueueRequired     = errors.New("rotate refresh: enqueue function is required")
)

const sessionAbsoluteMaxAge = 90 * 24 * time.Hour

func (r *InMemorySessionRepository) SeedRaw(s store.Session) store.Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[s.ID] = s
	return s
}
