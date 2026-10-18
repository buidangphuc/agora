// Package repository is team-identity's persistence boundary: users +
// credentials. Passwords are stored as bcrypt hashes; the port is swappable
// (Postgres in prod, in-memory for tests).
package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	// ErrNotFound is returned when a user lookup misses.
	ErrNotFound = errors.New("user not found")
	// ErrConflict is returned when a username already exists.
	ErrConflict = errors.New("username already exists")
	// ErrTokenNotFound is returned when a reset token lookup misses.
	ErrTokenNotFound = errors.New("reset token not found")
)

// User is the persistence-layer account.
type User struct {
	ID           string
	Username     string
	PasswordHash string
	Roles        []string

	// Enforcement state. Zero times map to NULL.
	Status          string
	StatusReason    string
	StatusUntil     time.Time
	StatusChangedAt time.Time
	CreatedAt       time.Time
}

// Account enforcement statuses.
const (
	StatusActive     = "active"
	StatusRestricted = "restricted"
	StatusSuspended  = "suspended"
	StatusBanned     = "banned"
)

// ErrAccountLocked is returned by CreateSession for a suspended or banned user.
var ErrAccountLocked = errors.New("account locked")

// StatusChange describes one committed status transition.
type StatusChange struct {
	UserID         string
	PreviousStatus string
	Status         string
	Reason         string
	Until          time.Time
	ChangedAt      time.Time
}

// StatusEnqueueFn builds the UserStatusChanged outbox row inside the write transaction.
type StatusEnqueueFn func(StatusChange) (OutboxRow, error)

// StatusResult is the outcome of SetStatus.
type StatusResult struct {
	User            User
	SessionsRevoked int
}

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

// PasswordResetToken represents a temporary token used for password reset.
type PasswordResetToken struct {
	TokenHash string
	UserID    string
	ExpiresAt time.Time
	Used      bool
	CreatedAt time.Time
}

// UserRepository is the port the service depends on.
type UserRepository interface {
	Create(ctx context.Context, u User) (User, error)
	GetByUsername(ctx context.Context, username string) (User, error)
	GetByID(ctx context.Context, id string) (User, error)
	UpdatePassword(ctx context.Context, userID, newPasswordHash string) error
	CreateResetToken(ctx context.Context, token PasswordResetToken) error
	GetResetToken(ctx context.Context, tokenHash string) (PasswordResetToken, error)
	MarkResetTokenUsed(ctx context.Context, tokenHash string) error
}

// ProfileReader is the narrow read port behind GetPublicProfiles. Unknown ids
// are simply absent from the result.
type ProfileReader interface {
	GetByIDs(ctx context.Context, ids []string) ([]User, error)
}

// InMemoryUserRepository is a fake store for tests.
type InMemoryUserRepository struct {
	mu     sync.RWMutex
	byName map[string]User
	byID   map[string]User
	tokens map[string]PasswordResetToken

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
		byName: make(map[string]User),
		byID:   make(map[string]User),
		tokens: make(map[string]PasswordResetToken),
	}
}

func (r *InMemoryUserRepository) Create(_ context.Context, u User) (User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byName[u.Username]; ok {
		return User{}, ErrConflict
	}
	if u.Status == "" {
		u.Status = StatusActive
	}
	r.byName[u.Username] = u
	r.byID[u.ID] = u
	return u, nil
}

func (r *InMemoryUserRepository) GetByUsername(_ context.Context, username string) (User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.byName[username]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

func (r *InMemoryUserRepository) GetByID(_ context.Context, id string) (User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.byID[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

func (r *InMemoryUserRepository) GetByIDs(_ context.Context, ids []string) ([]User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]User, 0, len(ids))
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
		return ErrNotFound
	}
	u.PasswordHash = newPasswordHash
	r.byID[userID] = u
	r.byName[u.Username] = u
	return nil
}

func (r *InMemoryUserRepository) CreateResetToken(_ context.Context, token PasswordResetToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[token.TokenHash] = token
	return nil
}

func (r *InMemoryUserRepository) GetResetToken(_ context.Context, tokenHash string) (PasswordResetToken, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	token, ok := r.tokens[tokenHash]
	if !ok {
		return PasswordResetToken{}, ErrTokenNotFound
	}
	return token, nil
}

func (r *InMemoryUserRepository) MarkResetTokenUsed(_ context.Context, tokenHash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	token, ok := r.tokens[tokenHash]
	if !ok {
		return ErrTokenNotFound
	}
	token.Used = true
	r.tokens[tokenHash] = token
	return nil
}

func (r *InMemoryUserRepository) store(u User) {
	r.byID[u.ID] = u
	r.byName[u.Username] = u
}

// ListUsers returns users whose id equals query or whose username starts with it,
// optionally filtered by status, ordered by username. The cursor is the last
// username of the previous page.
func (r *InMemoryUserRepository) ListUsers(_ context.Context, query, status, cursor string, limit int) ([]User, string, error) {
	limit = clampListLimit(limit)
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []User
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
func (r *InMemoryUserRepository) SetStatus(_ context.Context, userID, status, reason string, until, now time.Time, revoke EnqueueFn, changed StatusEnqueueFn) (StatusResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[userID]
	if !ok {
		return StatusResult{}, ErrNotFound
	}
	prev := u.Status
	var rows []OutboxRow
	revokedSessions := map[string]Session{}
	if r.sessions != nil {
		r.sessions.mu.Lock()
		defer r.sessions.mu.Unlock()
		if status != StatusActive {
			var live []Session
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
					return StatusResult{}, fmt.Errorf("build revoke event: %w", err)
				}
				rows = append(rows, row)
				revokedSessions[s.ID] = s
			}
		}
	}
	u.Status, u.StatusReason, u.StatusUntil, u.StatusChangedAt = status, reason, until, now
	row, err := changed(StatusChange{UserID: userID, PreviousStatus: prev, Status: status, Reason: reason, Until: until, ChangedAt: now})
	if err != nil {
		return StatusResult{}, fmt.Errorf("build status event: %w", err)
	}
	rows = append(rows, row)
	if r.sessions != nil {
		for id, s := range revokedSessions {
			r.sessions.sessions[id] = s
		}
		r.sessions.outbox = append(r.sessions.outbox, rows...)
	}
	r.store(u)
	return StatusResult{User: u, SessionsRevoked: len(revokedSessions)}, nil
}

// LiftExpiredSuspension returns a suspended user whose until has passed to active.
func (r *InMemoryUserRepository) LiftExpiredSuspension(_ context.Context, userID string, now time.Time, changed StatusEnqueueFn) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[userID]
	if !ok || u.Status != StatusSuspended || u.StatusUntil.IsZero() || u.StatusUntil.After(now) {
		return false, nil
	}
	row, err := changed(StatusChange{UserID: userID, PreviousStatus: StatusSuspended, Status: StatusActive, ChangedAt: now})
	if err != nil {
		return false, fmt.Errorf("build status event: %w", err)
	}
	u.Status, u.StatusReason, u.StatusUntil, u.StatusChangedAt = StatusActive, "", time.Time{}, now
	if r.sessions != nil {
		r.sessions.mu.Lock()
		r.sessions.outbox = append(r.sessions.outbox, row)
		r.sessions.mu.Unlock()
	}
	r.store(u)
	return true, nil
}

var (
	_ UserRepository = (*InMemoryUserRepository)(nil)
	_ ProfileReader  = (*InMemoryUserRepository)(nil)
)
