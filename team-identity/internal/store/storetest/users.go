package storetest

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/buidangphuc/team-identity/internal/store"
)

// InMemoryUserRepository is a fake store for tests.
type InMemoryUserRepository struct {
	mu     sync.RWMutex
	byName map[string]store.User
	byID   map[string]store.User
	tokens map[string]store.PasswordResetToken

	sessions *InMemorySessionRepository
}

// AttachSessions links this twin with a session twin (shared sessions, outbox and
// the CreateSession status gate). Lock order is always users then sessions.
func (r *InMemoryUserRepository) AttachSessions(s *InMemorySessionRepository) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions = s
	s.users = r
}

func NewInMemoryUserRepository() *InMemoryUserRepository {
	return &InMemoryUserRepository{
		byName: make(map[string]store.User),
		byID:   make(map[string]store.User),
		tokens: make(map[string]store.PasswordResetToken),
	}
}

func (r *InMemoryUserRepository) Create(_ context.Context, u store.User) (store.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byName[u.Username]; ok {
		return store.User{}, store.ErrConflict
	}
	if u.Status == "" {
		u.Status = store.StatusActive
	}
	r.byName[u.Username] = u
	r.byID[u.ID] = u
	return u, nil
}

func (r *InMemoryUserRepository) GetByUsername(_ context.Context, username string) (store.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.byName[username]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	return u, nil
}

func (r *InMemoryUserRepository) GetByID(_ context.Context, id string) (store.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.byID[id]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	return u, nil
}

func (r *InMemoryUserRepository) GetByIDs(_ context.Context, ids []string) ([]store.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]store.User, 0, len(ids))
	for _, id := range ids {
		if u, ok := r.byID[id]; ok {
			out = append(out, u)
		}
	}
	return out, nil
}

func (r *InMemoryUserRepository) UpdatePassword(_ context.Context, userID, newPasswordHash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[userID]
	if !ok {
		return store.ErrNotFound
	}
	u.PasswordHash = newPasswordHash
	r.byID[userID] = u
	r.byName[u.Username] = u
	return nil
}

func (r *InMemoryUserRepository) CreateResetToken(_ context.Context, token store.PasswordResetToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[token.TokenHash] = token
	return nil
}

func (r *InMemoryUserRepository) GetResetToken(_ context.Context, tokenHash string) (store.PasswordResetToken, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	token, ok := r.tokens[tokenHash]
	if !ok {
		return store.PasswordResetToken{}, store.ErrTokenNotFound
	}
	return token, nil
}

func (r *InMemoryUserRepository) MarkResetTokenUsed(_ context.Context, tokenHash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	token, ok := r.tokens[tokenHash]
	if !ok {
		return store.ErrTokenNotFound
	}
	token.Used = true
	r.tokens[tokenHash] = token
	return nil
}

func (r *InMemoryUserRepository) store(u store.User) {
	r.byID[u.ID] = u
	r.byName[u.Username] = u
}

// ListUsers returns users whose id equals query or whose username starts with it,
// optionally filtered by status, ordered by username. The cursor is the last
// username of the previous page.
func (r *InMemoryUserRepository) ListUsers(_ context.Context, query, status, cursor string, limit int) ([]store.User, string, error) {
	limit = clampListLimit(limit)
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []store.User
	for _, u := range r.byID {
		if query != "" && u.ID != query && !strings.HasPrefix(u.Username, query) {
			continue
		}
		if status != "" && u.Status != status {
			continue
		}
		if cursor != "" && u.Username <= cursor {
			continue
		}
		all = append(all, u)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Username < all[j].Username })
	if len(all) > limit {
		return all[:limit], all[limit-1].Username, nil
	}
	return all, "", nil
}

// SetStatus applies a status change. Non-active targets revoke every live session
// (one revoke event each), then one status event is built last. Nothing is
// committed unless every builder succeeds.
func (r *InMemoryUserRepository) SetStatus(_ context.Context, userID, status, reason string, until, now time.Time, revoke store.EnqueueFn, changed store.StatusEnqueueFn) (store.StatusResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[userID]
	if !ok {
		return store.StatusResult{}, store.ErrNotFound
	}
	prev := u.Status
	var rows []store.OutboxRow
	revokedSessions := map[string]store.Session{}
	if r.sessions != nil {
		r.sessions.mu.Lock()
		defer r.sessions.mu.Unlock()
		if status != store.StatusActive {
			var live []store.Session
			for _, s := range r.sessions.sessions {
				if s.UserID == userID && !s.Revoked {
					live = append(live, s)
				}
			}
			sort.Slice(live, func(i, j int) bool {
				if !live[i].CreatedAt.Equal(live[j].CreatedAt) {
					return live[i].CreatedAt.Before(live[j].CreatedAt)
				}
				return live[i].ID < live[j].ID
			})
			for _, s := range live {
				s.Revoked = true
				s.RefreshHash, s.PrevRefreshHash = "", ""
				row, err := revoke(s)
				if err != nil {
					return store.StatusResult{}, fmt.Errorf("build revoke event: %w", err)
				}
				rows = append(rows, row)
				revokedSessions[s.ID] = s
			}
		}
	}
	u.Status, u.StatusReason, u.StatusUntil, u.StatusChangedAt = status, reason, until, now
	row, err := changed(store.StatusChange{UserID: userID, PreviousStatus: prev, Status: status, Reason: reason, Until: until, ChangedAt: now})
	if err != nil {
		return store.StatusResult{}, fmt.Errorf("build status event: %w", err)
	}
	rows = append(rows, row)
	if r.sessions != nil {
		for id, s := range revokedSessions {
			r.sessions.sessions[id] = s
		}
		r.sessions.outbox = append(r.sessions.outbox, rows...)
	}
	r.store(u)
	return store.StatusResult{User: u, SessionsRevoked: len(revokedSessions)}, nil
}

// LiftExpiredSuspension returns a suspended user whose until has passed to active.
func (r *InMemoryUserRepository) LiftExpiredSuspension(_ context.Context, userID string, now time.Time, changed store.StatusEnqueueFn) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[userID]
	if !ok || u.Status != store.StatusSuspended || u.StatusUntil.IsZero() || u.StatusUntil.After(now) {
		return false, nil
	}
	row, err := changed(store.StatusChange{UserID: userID, PreviousStatus: store.StatusSuspended, Status: store.StatusActive, ChangedAt: now})
	if err != nil {
		return false, fmt.Errorf("build status event: %w", err)
	}
	u.Status, u.StatusReason, u.StatusUntil, u.StatusChangedAt = store.StatusActive, "", time.Time{}, now
	if r.sessions != nil {
		r.sessions.mu.Lock()
		r.sessions.outbox = append(r.sessions.outbox, row)
		r.sessions.mu.Unlock()
	}
	r.store(u)
	return true, nil
}

var (
	_ store.UserRepository = (*InMemoryUserRepository)(nil)
	_ store.ProfileReader  = (*InMemoryUserRepository)(nil)
)

const (
	defaultListLimit = 20
	maxListLimit     = 100
)

func clampListLimit(n int) int {
	if n <= 0 {
		return defaultListLimit
	}
	if n > maxListLimit {
		return maxListLimit
	}
	return n
}
