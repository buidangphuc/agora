package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrSessionNotFound is returned when a session does not exist or is not owned
// by the caller (the two are indistinguishable on purpose, to avoid leaking
// other users' session ids).
var ErrSessionNotFound = errors.New("session not found")

// Session is an active authenticated session (device/browser) for a user.
type Session struct {
	ID        string
	UserID    string
	Device    string
	IP        string
	CreatedAt time.Time
	LastSeen  time.Time
	Revoked   bool
}

// LoginEvent is a recorded login attempt (successful or failed) for a user.
type LoginEvent struct {
	ID        string
	UserID    string
	IP        string
	UserAgent string
	Success   bool
	CreatedAt time.Time
}

// SessionRepository manages a user's active sessions and login history. All
// reads/writes are scoped to the owning user_id — there is no cross-user access.
type SessionRepository interface {
	// CreateSession records a new active session for the user.
	CreateSession(ctx context.Context, s Session) (Session, error)
	// ListSessions returns the user's sessions, newest activity first.
	ListSessions(ctx context.Context, userID string) ([]Session, error)
	// RevokeSession marks the given session revoked; ErrSessionNotFound if the
	// session is missing or owned by another user.
	RevokeSession(ctx context.Context, sessionID, userID string) error
	// RecordLogin appends a login attempt to the user's history.
	RecordLogin(ctx context.Context, e LoginEvent) (LoginEvent, error)
	// ListLoginHistory returns a page of the user's login history (newest first)
	// plus the total number of events for that user.
	ListLoginHistory(ctx context.Context, userID string, limit, offset int) ([]LoginEvent, int64, error)
}

// --- Postgres implementation ---

type PostgresSessionRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresSessionRepository(pool *pgxpool.Pool) *PostgresSessionRepository {
	return &PostgresSessionRepository{pool: pool}
}

const sessionColumns = `id, user_id, device, ip, created_at, last_seen, revoked`

func scanSession(row pgx.Row, s *Session) error {
	return row.Scan(&s.ID, &s.UserID, &s.Device, &s.IP, &s.CreatedAt, &s.LastSeen, &s.Revoked)
}

func (r *PostgresSessionRepository) CreateSession(ctx context.Context, s Session) (Session, error) {
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	if s.LastSeen.IsZero() {
		s.LastSeen = s.CreatedAt
	}
	const q = `INSERT INTO sessions (id, user_id, device, ip, created_at, last_seen, revoked)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING ` + sessionColumns
	var out Session
	if err := scanSession(r.pool.QueryRow(ctx, q, s.ID, s.UserID, s.Device, s.IP, s.CreatedAt, s.LastSeen, s.Revoked), &out); err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	return out, nil
}

func (r *PostgresSessionRepository) ListSessions(ctx context.Context, userID string) ([]Session, error) {
	const q = `SELECT ` + sessionColumns + ` FROM sessions WHERE user_id = $1 ORDER BY last_seen DESC, created_at DESC`
	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var items []Session
	for rows.Next() {
		var s Session
		if err := scanSession(rows, &s); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		items = append(items, s)
	}
	return items, rows.Err()
}

func (r *PostgresSessionRepository) RevokeSession(ctx context.Context, sessionID, userID string) error {
	const q = `UPDATE sessions SET revoked = true WHERE id = $1 AND user_id = $2`
	res, err := r.pool.Exec(ctx, q, sessionID, userID)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (r *PostgresSessionRepository) RecordLogin(ctx context.Context, e LoginEvent) (LoginEvent, error) {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	const q = `INSERT INTO login_history (id, user_id, ip, user_agent, success, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, user_id, ip, user_agent, success, created_at`
	var out LoginEvent
	if err := r.pool.QueryRow(ctx, q, e.ID, e.UserID, e.IP, e.UserAgent, e.Success, e.CreatedAt).
		Scan(&out.ID, &out.UserID, &out.IP, &out.UserAgent, &out.Success, &out.CreatedAt); err != nil {
		return LoginEvent{}, fmt.Errorf("record login: %w", err)
	}
	return out, nil
}

func (r *PostgresSessionRepository) ListLoginHistory(ctx context.Context, userID string, limit, offset int) ([]LoginEvent, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM login_history WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count login history: %w", err)
	}

	const q = `SELECT id, user_id, ip, user_agent, success, created_at
		FROM login_history WHERE user_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, q, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list login history: %w", err)
	}
	defer rows.Close()

	var items []LoginEvent
	for rows.Next() {
		var e LoginEvent
		if err := rows.Scan(&e.ID, &e.UserID, &e.IP, &e.UserAgent, &e.Success, &e.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan login event: %w", err)
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// --- InMemory implementation (tests, no live DB) ---

type InMemorySessionRepository struct {
	mu       sync.RWMutex
	sessions map[string]Session
	logins   []LoginEvent
	seq      int64
}

func NewInMemorySessionRepository() *InMemorySessionRepository {
	return &InMemorySessionRepository{sessions: make(map[string]Session)}
}

func (r *InMemorySessionRepository) CreateSession(_ context.Context, s Session) (Session, error) {
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

func (r *InMemorySessionRepository) ListSessions(_ context.Context, userID string) ([]Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var items []Session
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

func (r *InMemorySessionRepository) RevokeSession(_ context.Context, sessionID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[sessionID]
	if !ok || s.UserID != userID {
		return ErrSessionNotFound
	}
	s.Revoked = true
	r.sessions[sessionID] = s
	return nil
}

func (r *InMemorySessionRepository) RecordLogin(_ context.Context, e LoginEvent) (LoginEvent, error) {
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

func (r *InMemorySessionRepository) ListLoginHistory(_ context.Context, userID string, limit, offset int) ([]LoginEvent, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var owned []LoginEvent
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
