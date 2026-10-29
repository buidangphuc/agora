package store

import (
	"context"
	"errors"
	"time"

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

type PostgresSavedSearchRepository struct{}

func NewPostgresSavedSearchRepository(_ *pgxpool.Pool) *PostgresSavedSearchRepository {
	return &PostgresSavedSearchRepository{}
}

func (*PostgresSavedSearchRepository) Create(context.Context, SavedSearch) (SavedSearch, error) {
	return SavedSearch{}, nil
}

func (*PostgresSavedSearchRepository) List(context.Context, string, int, int) ([]SavedSearch, int64, error) {
	return nil, 0, nil
}

func (*PostgresSavedSearchRepository) Get(context.Context, string, string) (SavedSearch, error) {
	return SavedSearch{}, nil
}

func (*PostgresSavedSearchRepository) Delete(context.Context, string, string) error { return nil }
