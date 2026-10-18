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
	"github.com/jackc/pgx/v5/pgconn"
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

	// Refresh-token state. Only SHA-256 hashes are stored. "" / zero time map to NULL.
	RefreshHash      string
	PrevRefreshHash  string
	RefreshExpiresAt time.Time
	RotatedAt        time.Time

	// UserStatus is the owning user's status read under lock by CreateSession; not persisted.
	UserStatus string
}

// RefreshTTL is the sliding lifetime of a refresh token, extended on each rotation.
const RefreshTTL = 30 * 24 * time.Hour

// RotateOutcome is the result class of RotateRefresh.
type RotateOutcome int

const (
	// RotateRotated: the live token was presented and replaced by the new hash.
	RotateRotated RotateOutcome = iota + 1
	// RotateReplayedInGrace: the previous token was replayed within the grace window; no change.
	RotateReplayedInGrace
	// RotateStale: the hash matches no live session; no change.
	RotateStale
	// RotateReuse: the previous token was replayed after the grace window; the session is revoked.
	RotateReuse
	// RotateExpired: the refresh token is past refresh_expires_at; no change.
	RotateExpired
	// RotateLifetimeExceeded: the session is older than the absolute max age; it is revoked.
	RotateLifetimeExceeded
)

// errRefreshHashConflict is returned when the new refresh hash collides with another
// session's hash. It deliberately carries no hash value.
var errRefreshHashConflict = errors.New("refresh hash conflict")

// ErrRotateFault marks a RotateRefresh/RevokeSession failure caused by event
// building, outbox enqueue or a refresh_hash conflict (not a connectivity error).
var ErrRotateFault = errors.New("rotate fault")

// errEnqueueRequired is returned when RotateRefresh is called without an outbox builder.
var errEnqueueRequired = errors.New("rotate refresh: enqueue function is required")

// RotateResult carries the outcome and the (post-operation) session, if one matched.
type RotateResult struct {
	Outcome RotateOutcome
	Session Session
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
	// session is missing or owned by another user. When ev is non-nil the outbox
	// row it builds from the revoked session is inserted in the SAME transaction:
	// if building or writing it fails, the revoke is rolled back.
	RevokeSession(ctx context.Context, sessionID, userID string, ev EnqueueFn) error
	// RotateRefresh atomically validates and rotates a refresh token (see RotateOutcome).
	// Reuse and absolute-lifetime expiry (now-created_at > maxAge) revoke the session, clear
	// both hashes and insert the outbox row built by ev in the same transaction (any error
	// leaves the session unchanged). ev must be non-nil. A token past refresh_expires_at
	// yields RotateExpired with no write.
	RotateRefresh(ctx context.Context, presentedHash, newHash string, now time.Time, grace, maxAge time.Duration, ip, device string, ev EnqueueFn) (RotateResult, error)
	// RecordLogin appends a login attempt to the user's history.
	RecordLogin(ctx context.Context, e LoginEvent) (LoginEvent, error)
	// ListLoginHistory returns a page of the user's login history (newest first)
	// plus the total number of events for that user.
	ListLoginHistory(ctx context.Context, userID string, limit, offset int) ([]LoginEvent, int64, error)
	// PurgeSessions deletes up to limit sessions that ended (revoked, refresh-expired or past the
	// absolute max age) more than retention before now; it returns the number deleted.
	PurgeSessions(ctx context.Context, now time.Time, retention time.Duration, limit int) (int, error)
	// PurgeLoginHistory deletes up to limit login events created strictly before the cutoff.
	PurgeLoginHistory(ctx context.Context, before time.Time, limit int) (int, error)
}

// sessionAbsoluteMaxAge is the absolute session lifetime counted as expiry by the purge.
const sessionAbsoluteMaxAge = 90 * 24 * time.Hour

// --- Postgres implementation ---

type PostgresSessionRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresSessionRepository(pool *pgxpool.Pool) *PostgresSessionRepository {
	return &PostgresSessionRepository{pool: pool}
}

const sessionColumns = `id, user_id, device, ip, created_at, last_seen, revoked,
	COALESCE(refresh_hash, ''), COALESCE(prev_refresh_hash, ''), refresh_expires_at, rotated_at`

func scanSession(row pgx.Row, s *Session) error {
	var exp, rot *time.Time
	if err := row.Scan(&s.ID, &s.UserID, &s.Device, &s.IP, &s.CreatedAt, &s.LastSeen, &s.Revoked,
		&s.RefreshHash, &s.PrevRefreshHash, &exp, &rot); err != nil {
		return err
	}
	s.RefreshExpiresAt, s.RotatedAt = time.Time{}, time.Time{}
	if exp != nil {
		s.RefreshExpiresAt = *exp
	}
	if rot != nil {
		s.RotatedAt = *rot
	}
	return nil
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
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
	const q = `INSERT INTO sessions (id, user_id, device, ip, created_at, last_seen, revoked, refresh_hash, refresh_expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9)
		RETURNING ` + sessionColumns
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Session{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Share-lock the user row so a concurrent SetStatus (FOR UPDATE) either
	// finishes first (and this insert is refused) or sees and revokes this session.
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM users WHERE id = $1 FOR SHARE`, s.UserID).Scan(&status); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Session{}, fmt.Errorf("lock user for session: %w", err)
	}
	if status == StatusSuspended || status == StatusBanned {
		return Session{}, ErrAccountLocked
	}
	var out Session
	if err := scanSession(tx.QueryRow(ctx, q, s.ID, s.UserID, s.Device, s.IP, s.CreatedAt, s.LastSeen, s.Revoked,
		s.RefreshHash, nullTime(s.RefreshExpiresAt)), &out); err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Session{}, fmt.Errorf("commit tx: %w", err)
	}
	out.UserStatus = status
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

func (r *PostgresSessionRepository) RevokeSession(ctx context.Context, sessionID, userID string, ev EnqueueFn) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `UPDATE sessions SET revoked = true, revoked_at = now(), refresh_hash = NULL, prev_refresh_hash = NULL WHERE id = $1 AND user_id = $2 RETURNING ` + sessionColumns
	var s Session
	if err := scanSession(tx.QueryRow(ctx, q, sessionID, userID), &s); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSessionNotFound
		}
		return fmt.Errorf("revoke session: %w", err)
	}
	if ev != nil {
		row, err := ev(s)
		if err != nil {
			return fmt.Errorf("build revoke event: %w: %w", ErrRotateFault, err)
		}
		if err := enqueueTx(ctx, tx, row); err != nil {
			return fmt.Errorf("enqueue revoke event: %w: %w", ErrRotateFault, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (r *PostgresSessionRepository) RotateRefresh(ctx context.Context, presentedHash, newHash string, now time.Time, grace, maxAge time.Duration, ip, device string, ev EnqueueFn) (RotateResult, error) {
	if ev == nil {
		return RotateResult{}, errEnqueueRequired
	}
	if presentedHash == "" {
		return RotateResult{Outcome: RotateStale}, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return RotateResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const sel = `SELECT ` + sessionColumns + ` FROM sessions
		WHERE (refresh_hash = $1 OR prev_refresh_hash = $1) AND NOT revoked FOR UPDATE`
	var s Session
	if err := scanSession(tx.QueryRow(ctx, sel, presentedHash), &s); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RotateResult{Outcome: RotateStale}, nil
		}
		return RotateResult{}, fmt.Errorf("select session for refresh: %w", err)
	}

	revoke := func(outcome RotateOutcome, msg string) (RotateResult, error) {
		const rev = `UPDATE sessions SET revoked = true, revoked_at = now(), refresh_hash = NULL, prev_refresh_hash = NULL
			WHERE id = $1 RETURNING ` + sessionColumns
		var out Session
		if err := scanSession(tx.QueryRow(ctx, rev, s.ID), &out); err != nil {
			return RotateResult{}, fmt.Errorf("%s: %w", msg, err)
		}
		row, err := ev(out)
		if err != nil {
			return RotateResult{}, fmt.Errorf("build revoke event: %w: %w", ErrRotateFault, err)
		}
		if err := enqueueTx(ctx, tx, row); err != nil {
			return RotateResult{}, fmt.Errorf("enqueue revoke event: %w: %w", ErrRotateFault, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return RotateResult{}, fmt.Errorf("commit tx: %w", err)
		}
		return RotateResult{Outcome: outcome, Session: out}, nil
	}

	if now.Sub(s.CreatedAt) > maxAge {
		return revoke(RotateLifetimeExceeded, "revoke on lifetime exceeded")
	}
	if !s.RefreshExpiresAt.IsZero() && !now.Before(s.RefreshExpiresAt) {
		return RotateResult{Outcome: RotateExpired, Session: s}, nil
	}

	switch {
	case s.RefreshHash == presentedHash:
		const upd = `UPDATE sessions SET prev_refresh_hash = refresh_hash, refresh_hash = $2,
			rotated_at = $3, last_seen = $3, refresh_expires_at = $4,
			ip = COALESCE(NULLIF($5, ''), ip), device = COALESCE(NULLIF($6, ''), device)
			WHERE id = $1 RETURNING ` + sessionColumns
		var out Session
		if err := scanSession(tx.QueryRow(ctx, upd, s.ID, newHash, now, now.Add(RefreshTTL), ip, device), &out); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return RotateResult{}, fmt.Errorf("%w: %w", errRefreshHashConflict, ErrRotateFault)
			}
			return RotateResult{}, fmt.Errorf("rotate refresh: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return RotateResult{}, fmt.Errorf("commit tx: %w", err)
		}
		return RotateResult{Outcome: RotateRotated, Session: out}, nil
	case !s.RotatedAt.IsZero() && now.Sub(s.RotatedAt) <= grace:
		return RotateResult{Outcome: RotateReplayedInGrace, Session: s}, nil
	default:
		return revoke(RotateReuse, "revoke on reuse")
	}
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

func (r *PostgresSessionRepository) PurgeSessions(ctx context.Context, now time.Time, retention time.Duration, limit int) (int, error) {
	const q = `DELETE FROM sessions WHERE id IN (
		SELECT id FROM sessions
		WHERE (revoked AND COALESCE(revoked_at, last_seen) < $1)
		   OR (NOT revoked AND (refresh_expires_at < $1 OR created_at < $2))
		ORDER BY id LIMIT $3 FOR UPDATE SKIP LOCKED)`
	tag, err := r.pool.Exec(ctx, q, now.Add(-retention), now.Add(-retention-sessionAbsoluteMaxAge), limit)
	if err != nil {
		return 0, fmt.Errorf("purge sessions: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (r *PostgresSessionRepository) PurgeLoginHistory(ctx context.Context, before time.Time, limit int) (int, error) {
	const q = `DELETE FROM login_history WHERE id IN (
		SELECT id FROM login_history WHERE created_at < $1 ORDER BY id LIMIT $2 FOR UPDATE SKIP LOCKED)`
	tag, err := r.pool.Exec(ctx, q, before, limit)
	if err != nil {
		return 0, fmt.Errorf("purge login history: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// --- InMemory implementation (tests, no live DB) ---

type InMemorySessionRepository struct {
	mu       sync.RWMutex
	sessions map[string]Session
	logins   []LoginEvent
	seq      int64
	outbox   []OutboxRow
	users    *InMemoryUserRepository // set by AttachSessions

	// FailOutbox makes the outbox write of a revoke fail, exercising the
	// rollback path (the session must stay un-revoked and no row is recorded).
	FailOutbox bool
}

func NewInMemorySessionRepository() *InMemorySessionRepository {
	return &InMemorySessionRepository{sessions: make(map[string]Session)}
}

func (r *InMemorySessionRepository) CreateSession(_ context.Context, s Session) (Session, error) {
	if r.users != nil {
		// Lock order: users then sessions (same as SetStatus).
		r.users.mu.RLock()
		defer r.users.mu.RUnlock()
		if u, ok := r.users.byID[s.UserID]; ok {
			if u.Status == StatusSuspended || u.Status == StatusBanned {
				return Session{}, ErrAccountLocked
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

func (r *InMemorySessionRepository) RevokeSession(_ context.Context, sessionID, userID string, ev EnqueueFn) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[sessionID]
	if !ok || s.UserID != userID {
		return ErrSessionNotFound
	}
	s.Revoked = true
	s.RefreshHash, s.PrevRefreshHash = "", ""
	// Mirror the Postgres transaction: build/record the outbox row first and only
	// then commit the revoke, so a failing outbox write leaves the session intact.
	if ev != nil {
		row, err := ev(s)
		if err != nil {
			return fmt.Errorf("build revoke event: %w: %w", ErrRotateFault, err)
		}
		if r.FailOutbox {
			return fmt.Errorf("enqueue revoke event: %w", ErrRotateFault)
		}
		r.outbox = append(r.outbox, row)
	}
	r.sessions[sessionID] = s
	return nil
}

func (r *InMemorySessionRepository) RotateRefresh(_ context.Context, presentedHash, newHash string, now time.Time, grace, maxAge time.Duration, ip, device string, ev EnqueueFn) (RotateResult, error) {
	if ev == nil {
		return RotateResult{}, errEnqueueRequired
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if presentedHash == "" {
		return RotateResult{Outcome: RotateStale}, nil
	}
	var s Session
	found := false
	for _, c := range r.sessions {
		if !c.Revoked && (c.RefreshHash == presentedHash || c.PrevRefreshHash == presentedHash) {
			s, found = c, true
			break
		}
	}
	if !found {
		return RotateResult{Outcome: RotateStale}, nil
	}
	revoke := func(outcome RotateOutcome) (RotateResult, error) {
		s.Revoked = true
		s.RefreshHash, s.PrevRefreshHash = "", ""
		row, err := ev(s)
		if err != nil {
			return RotateResult{}, fmt.Errorf("build revoke event: %w: %w", ErrRotateFault, err)
		}
		if r.FailOutbox {
			return RotateResult{}, fmt.Errorf("enqueue revoke event: %w", ErrRotateFault)
		}
		r.outbox = append(r.outbox, row)
		r.sessions[s.ID] = s
		return RotateResult{Outcome: outcome, Session: s}, nil
	}
	if now.Sub(s.CreatedAt) > maxAge {
		return revoke(RotateLifetimeExceeded)
	}
	if !s.RefreshExpiresAt.IsZero() && !now.Before(s.RefreshExpiresAt) {
		return RotateResult{Outcome: RotateExpired, Session: s}, nil
	}
	switch {
	case s.RefreshHash == presentedHash:
		for _, c := range r.sessions {
			if c.ID != s.ID && (c.RefreshHash == newHash) && newHash != "" {
				return RotateResult{}, fmt.Errorf("%w: %w", errRefreshHashConflict, ErrRotateFault)
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
		s.RefreshExpiresAt = now.Add(RefreshTTL)
		r.sessions[s.ID] = s
		return RotateResult{Outcome: RotateRotated, Session: s}, nil
	case !s.RotatedAt.IsZero() && now.Sub(s.RotatedAt) <= grace:
		return RotateResult{Outcome: RotateReplayedInGrace, Session: s}, nil
	default:
		return revoke(RotateReuse)
	}
}

// OutboxRows returns a copy of the outbox rows recorded by revokes, for assertions.
func (r *InMemorySessionRepository) OutboxRows() []OutboxRow {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]OutboxRow(nil), r.outbox...)
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
	kept := make([]LoginEvent, 0, len(r.logins))
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
