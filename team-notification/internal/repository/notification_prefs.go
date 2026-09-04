package repository

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
)

// ErrPrefsNotFound is returned by Get when a user has never set preferences.
// The service treats it as "use defaults" rather than an error.
var ErrPrefsNotFound = errors.New("notification prefs not found")

// NotificationPrefsRepository stores each user's per-type opt-in map plus their
// digest cadence. One row per user; owned by team-notification. Both a Postgres
// and an in-memory implementation satisfy this.
type NotificationPrefsRepository interface {
	// Get returns the stored preferences for a user, or ErrPrefsNotFound when
	// none have been set (the service fills in defaults in that case).
	Get(ctx context.Context, userID string) (*notificationv1.NotificationPrefs, error)
	// Upsert replaces the user's preferences, returning the stored value.
	Upsert(ctx context.Context, userID string, prefs *notificationv1.NotificationPrefs) (*notificationv1.NotificationPrefs, error)
	// ListUsersByDigestFreq returns the ids of users whose digest cadence matches
	// freq. The digest scheduler runs this query to find who to bundle for a run.
	ListUsersByDigestFreq(ctx context.Context, freq notificationv1.DigestFrequency) ([]string, error)
}

// ── Postgres implementation ──

type PostgresNotificationPrefsRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresNotificationPrefsRepo(pool *pgxpool.Pool) *PostgresNotificationPrefsRepo {
	return &PostgresNotificationPrefsRepo{pool: pool}
}

func (r *PostgresNotificationPrefsRepo) Get(ctx context.Context, userID string) (*notificationv1.NotificationPrefs, error) {
	var raw []byte
	var freq int
	err := r.pool.QueryRow(ctx, `
		SELECT prefs, digest_freq
		FROM notification_prefs
		WHERE user_id = $1`, userID).Scan(&raw, &freq)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPrefsNotFound
	}
	if err != nil {
		return nil, err
	}
	typeEnabled := map[string]bool{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &typeEnabled); err != nil {
			return nil, err
		}
	}
	return &notificationv1.NotificationPrefs{
		TypeEnabled: typeEnabled,
		DigestFreq:  notificationv1.DigestFrequency(freq),
	}, nil
}

func (r *PostgresNotificationPrefsRepo) Upsert(ctx context.Context, userID string, prefs *notificationv1.NotificationPrefs) (*notificationv1.NotificationPrefs, error) {
	raw, err := json.Marshal(prefs.GetTypeEnabled())
	if err != nil {
		return nil, err
	}
	// One row per user: re-saving preferences overwrites the previous ones.
	const q = `
		INSERT INTO notification_prefs (user_id, prefs, digest_freq, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (user_id)
		DO UPDATE SET prefs = EXCLUDED.prefs, digest_freq = EXCLUDED.digest_freq, updated_at = NOW()`
	if _, err := r.pool.Exec(ctx, q, userID, raw, int(prefs.GetDigestFreq())); err != nil {
		return nil, err
	}
	return clonePrefs(prefs), nil
}

func (r *PostgresNotificationPrefsRepo) ListUsersByDigestFreq(ctx context.Context, freq notificationv1.DigestFrequency) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT user_id
		FROM notification_prefs
		WHERE digest_freq = $1
		ORDER BY user_id`, int(freq))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// ── In-memory implementation ──

type InMemoryNotificationPrefsRepo struct {
	mu       sync.Mutex
	byUserID map[string]*notificationv1.NotificationPrefs
}

func NewInMemoryNotificationPrefsRepo() *InMemoryNotificationPrefsRepo {
	return &InMemoryNotificationPrefsRepo{byUserID: make(map[string]*notificationv1.NotificationPrefs)}
}

func (r *InMemoryNotificationPrefsRepo) Get(_ context.Context, userID string) (*notificationv1.NotificationPrefs, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byUserID[userID]
	if !ok {
		return nil, ErrPrefsNotFound
	}
	return clonePrefs(p), nil
}

func (r *InMemoryNotificationPrefsRepo) Upsert(_ context.Context, userID string, prefs *notificationv1.NotificationPrefs) (*notificationv1.NotificationPrefs, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored := clonePrefs(prefs)
	r.byUserID[userID] = stored
	return clonePrefs(stored), nil
}

func (r *InMemoryNotificationPrefsRepo) ListUsersByDigestFreq(_ context.Context, freq notificationv1.DigestFrequency) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var users []string
	for id, p := range r.byUserID {
		if p.GetDigestFreq() == freq {
			users = append(users, id)
		}
	}
	sort.Strings(users)
	return users, nil
}

func clonePrefs(p *notificationv1.NotificationPrefs) *notificationv1.NotificationPrefs {
	te := make(map[string]bool, len(p.GetTypeEnabled()))
	for k, v := range p.GetTypeEnabled() {
		te[k] = v
	}
	return &notificationv1.NotificationPrefs{
		TypeEnabled: te,
		DigestFreq:  p.GetDigestFreq(),
	}
}
