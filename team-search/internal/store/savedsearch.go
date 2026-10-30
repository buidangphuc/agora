package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSavedSearchNotFound = errors.New("saved search not found")

type SavedSearch struct {
	ID          string
	UserID      string
	Query       string
	FiltersJSON string
	CreatedAt   time.Time
}

type SavedSearchRepository interface {
	Create(ctx context.Context, s SavedSearch) (SavedSearch, error)
	List(ctx context.Context, userID string, limit, offset int) ([]SavedSearch, int64, error)
	Get(ctx context.Context, id, userID string) (SavedSearch, error)
	Delete(ctx context.Context, id, userID string) error
}

type PostgresSavedSearchRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresSavedSearchRepository(pool *pgxpool.Pool) *PostgresSavedSearchRepository {
	return &PostgresSavedSearchRepository{pool: pool}
}

const savedSearchColumns = `id, user_id, query, filters::text, created_at`

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
	out, err := scanSavedSearch(r.pool.QueryRow(ctx, q, s.ID, s.UserID, s.Query, s.FiltersJSON))
	if err != nil {
		return SavedSearch{}, fmt.Errorf("create saved search: %w", err)
	}
	return out, nil
}

func (r *PostgresSavedSearchRepository) List(ctx context.Context, userID string, limit, offset int) ([]SavedSearch, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM saved_searches WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count saved searches: %w", err)
	}
	const q = `SELECT ` + savedSearchColumns + ` FROM saved_searches
		WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, q, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list saved searches: %w", err)
	}
	defer rows.Close()
	items := make([]SavedSearch, 0, max(limit, 0))
	for rows.Next() {
		s, err := scanSavedSearch(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan saved search: %w", err)
		}
		items = append(items, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list saved searches: %w", err)
	}
	return items, total, nil
}

func (r *PostgresSavedSearchRepository) Get(ctx context.Context, id, userID string) (SavedSearch, error) {
	const q = `SELECT ` + savedSearchColumns + ` FROM saved_searches WHERE id = $1 AND user_id = $2`
	s, err := scanSavedSearch(r.pool.QueryRow(ctx, q, id, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SavedSearch{}, ErrSavedSearchNotFound
		}
		return SavedSearch{}, fmt.Errorf("get saved search %q: %w", id, err)
	}
	return s, nil
}

func (r *PostgresSavedSearchRepository) Delete(ctx context.Context, id, userID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM saved_searches WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete saved search %q: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSavedSearchNotFound
	}
	return nil
}

func scanSavedSearch(row pgx.Row) (SavedSearch, error) {
	var s SavedSearch
	err := row.Scan(&s.ID, &s.UserID, &s.Query, &s.FiltersJSON, &s.CreatedAt)
	return s, err
}
