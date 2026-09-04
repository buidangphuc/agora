package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrCategoryNotFound is returned when a category lookup misses.
var ErrCategoryNotFound = errors.New("category not found")

// Category is the domain model for product categories.
type Category struct {
	ID           string
	Name         string
	Slug         string
	ParentID     string
	DisplayOrder int32
	IconURL      string
}

// CategoryRepository defines access to product category taxonomy.
type CategoryRepository interface {
	List(ctx context.Context, parentID string) ([]Category, error)
	Get(ctx context.Context, id string) (Category, error)
}

// PostgresCategoryRepository implements CategoryRepository on Postgres.
type PostgresCategoryRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresCategoryRepository builds a category repository over pgxpool.
func NewPostgresCategoryRepository(pool *pgxpool.Pool) *PostgresCategoryRepository {
	return &PostgresCategoryRepository{pool: pool}
}

const categoryColumns = `id, name, slug, COALESCE(parent_id, ''), display_order, icon_url`

func scanCategory(row pgx.Row, c *Category) error {
	return row.Scan(&c.ID, &c.Name, &c.Slug, &c.ParentID, &c.DisplayOrder, &c.IconURL)
}

func (r *PostgresCategoryRepository) List(ctx context.Context, parentID string) ([]Category, error) {
	var q string
	var rows pgx.Rows
	var err error

	if parentID == "" {
		q = `SELECT ` + categoryColumns + ` FROM categories WHERE parent_id IS NULL ORDER BY display_order, name`
		rows, err = r.pool.Query(ctx, q)
	} else {
		q = `SELECT ` + categoryColumns + ` FROM categories WHERE parent_id = $1 ORDER BY display_order, name`
		rows, err = r.pool.Query(ctx, q, parentID)
	}
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()

	var items []Category
	for rows.Next() {
		var c Category
		if err := scanCategory(rows, &c); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

func (r *PostgresCategoryRepository) Get(ctx context.Context, id string) (Category, error) {
	const q = `SELECT ` + categoryColumns + ` FROM categories WHERE id = $1`
	var c Category
	if err := scanCategory(r.pool.QueryRow(ctx, q, id), &c); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return Category{}, ErrCategoryNotFound
		}
		return Category{}, fmt.Errorf("get category %q: %w", id, err)
	}
	return c, nil
}

// InMemoryCategoryRepository is a deterministic fake store for tests.
type InMemoryCategoryRepository struct {
	mu   sync.RWMutex
	byID map[string]Category
}

// NewInMemoryCategoryRepository seeds standard categories for unit testing.
func NewInMemoryCategoryRepository(seed ...Category) *InMemoryCategoryRepository {
	if len(seed) == 0 {
		seed = []Category{
			{ID: "cat-electronics", Name: "Điện tử & Công nghệ", Slug: "dien-tu", DisplayOrder: 1, IconURL: "📱"},
			{ID: "cat-fashion", Name: "Thời trang & Phụ kiện", Slug: "thoi-trang", DisplayOrder: 2, IconURL: "👕"},
			{ID: "cat-home", Name: "Nhà cửa & Đời sống", Slug: "nha-cua", DisplayOrder: 3, IconURL: "🏠"},
		}
	}
	byID := make(map[string]Category, len(seed))
	for _, c := range seed {
		byID[c.ID] = c
	}
	return &InMemoryCategoryRepository{byID: byID}
}

func (r *InMemoryCategoryRepository) List(_ context.Context, parentID string) ([]Category, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var items []Category
	for _, c := range r.byID {
		if c.ParentID == parentID {
			items = append(items, c)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].DisplayOrder == items[j].DisplayOrder {
			return items[i].Name < items[j].Name
		}
		return items[i].DisplayOrder < items[j].DisplayOrder
	})
	return items, nil
}

func (r *InMemoryCategoryRepository) Get(_ context.Context, id string) (Category, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.byID[id]
	if !ok {
		return Category{}, ErrCategoryNotFound
	}
	return c, nil
}
