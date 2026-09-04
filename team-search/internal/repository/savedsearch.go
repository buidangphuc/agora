// Package repository is team-search's OWN persistence for user-scoped saved
// searches: a small relational store that sits ALONGSIDE the OpenSearch
// read-model (which stays the query engine). Saved searches are personal state
// (query + filters a user parks to re-run later), so they live in Postgres, not
// the rebuildable index. Two implementations back the SavedSearchRepository
// port: a Postgres one for production and an in-memory one so handlers/tests run
// without a live DB.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ErrSavedSearchNotFound is returned when no saved search matches the id for the
// requesting user (either it never existed or it belongs to someone else — the
// two are deliberately indistinguishable so ownership can't be probed).
var ErrSavedSearchNotFound = errors.New("saved search not found")

// SavedSearch is a persisted query + opaque filters JSON, owned by a user.
// FiltersJSON mirrors SearchListingsRequest.filters (a JSON object of
// string->string term filters); it is stored opaquely and re-parsed at run time.
type SavedSearch struct {
	ID          string
	UserID      string
	Query       string
	FiltersJSON string
	CreatedAt   time.Time
}

// SavedSearchRepository is the persistence port. Every method is self-scoped by
// userID so one user can never read or delete another's saved searches.
type SavedSearchRepository interface {
	Create(ctx context.Context, s SavedSearch) (SavedSearch, error)
	// List returns one page (newest first) of the user's saved searches plus the
	// total count for pagination.
	List(ctx context.Context, userID string, limit, offset int) ([]SavedSearch, int64, error)
	Get(ctx context.Context, id, userID string) (SavedSearch, error)
	Delete(ctx context.Context, id, userID string) error
}

// ── Postgres implementation ─────────────────────────────────────────────────

// PostgresSavedSearchRepository stores saved searches in Postgres via the
// stdlib database/sql interface, so no concrete driver is imported here (and no
// new module dependency is pulled in); the driver is registered by the caller
// that opens the *sql.DB.
type PostgresSavedSearchRepository struct {
	db *sql.DB
}

// NewPostgresSavedSearchRepository wires the repo around an open *sql.DB.
func NewPostgresSavedSearchRepository(db *sql.DB) *PostgresSavedSearchRepository {
	return &PostgresSavedSearchRepository{db: db}
}

const savedSearchColumns = `id, user_id, query, filters, created_at`

func (r *PostgresSavedSearchRepository) Create(ctx context.Context, s SavedSearch) (SavedSearch, error) {
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if s.FiltersJSON == "" {
		s.FiltersJSON = "{}"
	}
	const q = `INSERT INTO saved_searches (id, user_id, query, filters, created_at)
		VALUES ($1, $2, $3, $4::jsonb, now())
		RETURNING ` + savedSearchColumns
	var out SavedSearch
	if err := scanSavedSearch(r.db.QueryRowContext(ctx, q, s.ID, s.UserID, s.Query, s.FiltersJSON), &out); err != nil {
		return SavedSearch{}, fmt.Errorf("create saved search: %w", err)
	}
	return out, nil
}

func (r *PostgresSavedSearchRepository) List(ctx context.Context, userID string, limit, offset int) ([]SavedSearch, int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM saved_searches WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count saved searches: %w", err)
	}
	const q = `SELECT ` + savedSearchColumns + ` FROM saved_searches
		WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`
	rows, err := r.db.QueryContext(ctx, q, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list saved searches: %w", err)
	}
	defer rows.Close()
	items := make([]SavedSearch, 0, limit)
	for rows.Next() {
		var s SavedSearch
		if err := scanSavedSearch(rows, &s); err != nil {
			return nil, 0, fmt.Errorf("scan saved search: %w", err)
		}
		items = append(items, s)
	}
	return items, total, rows.Err()
}

func (r *PostgresSavedSearchRepository) Get(ctx context.Context, id, userID string) (SavedSearch, error) {
	const q = `SELECT ` + savedSearchColumns + ` FROM saved_searches WHERE id = $1 AND user_id = $2`
	var s SavedSearch
	if err := scanSavedSearch(r.db.QueryRowContext(ctx, q, id, userID), &s); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SavedSearch{}, ErrSavedSearchNotFound
		}
		return SavedSearch{}, fmt.Errorf("get saved search %q: %w", id, err)
	}
	return s, nil
}

func (r *PostgresSavedSearchRepository) Delete(ctx context.Context, id, userID string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM saved_searches WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete saved search %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete saved search %q: %w", id, err)
	}
	if n == 0 {
		return ErrSavedSearchNotFound
	}
	return nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanSavedSearch(row rowScanner, s *SavedSearch) error {
	return row.Scan(&s.ID, &s.UserID, &s.Query, &s.FiltersJSON, &s.CreatedAt)
}

// ── In-memory implementation ────────────────────────────────────────────────

// InMemorySavedSearchRepository is a deterministic fake store for tests and for
// running the service without Postgres. A monotonic seq gives a stable
// newest-first order even when CreatedAt timestamps collide.
type InMemorySavedSearchRepository struct {
	mu   sync.RWMutex
	byID map[string]SavedSearch
	seq  map[string]int64
	next int64
	now  func() time.Time
}

// NewInMemorySavedSearchRepository builds an empty in-memory store.
func NewInMemorySavedSearchRepository() *InMemorySavedSearchRepository {
	return &InMemorySavedSearchRepository{
		byID: make(map[string]SavedSearch),
		seq:  make(map[string]int64),
		now:  time.Now,
	}
}

func (r *InMemorySavedSearchRepository) Create(_ context.Context, s SavedSearch) (SavedSearch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	if s.FiltersJSON == "" {
		s.FiltersJSON = "{}"
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = r.now().UTC()
	}
	r.byID[s.ID] = s
	r.next++
	r.seq[s.ID] = r.next
	return s, nil
}

func (r *InMemorySavedSearchRepository) List(_ context.Context, userID string, limit, offset int) ([]SavedSearch, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	owned := make([]SavedSearch, 0, len(r.byID))
	for _, s := range r.byID {
		if s.UserID == userID {
			owned = append(owned, s)
		}
	}
	// Newest first: higher insertion seq wins.
	sort.Slice(owned, func(i, j int) bool {
		return r.seq[owned[i].ID] > r.seq[owned[j].ID]
	})
	total := int64(len(owned))
	if offset < 0 {
		offset = 0
	}
	if offset >= len(owned) {
		return []SavedSearch{}, total, nil
	}
	end := len(owned)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return append([]SavedSearch(nil), owned[offset:end]...), total, nil
}

func (r *InMemorySavedSearchRepository) Get(_ context.Context, id, userID string) (SavedSearch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byID[id]
	if !ok || s.UserID != userID {
		return SavedSearch{}, ErrSavedSearchNotFound
	}
	return s, nil
}

func (r *InMemorySavedSearchRepository) Delete(_ context.Context, id, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byID[id]
	if !ok || s.UserID != userID {
		return ErrSavedSearchNotFound
	}
	delete(r.byID, id)
	delete(r.seq, id)
	return nil
}

// compile-time assertions.
var (
	_ SavedSearchRepository = (*PostgresSavedSearchRepository)(nil)
	_ SavedSearchRepository = (*InMemorySavedSearchRepository)(nil)
)
