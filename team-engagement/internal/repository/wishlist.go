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

// ErrCollectionNotFound is returned when a collection does not exist or is not
// owned by the requesting user.
var ErrCollectionNotFound = errors.New("collection not found")

// Collection is a per-user named wishlist (F3).
type Collection struct {
	ID        string
	UserID    string
	Name      string
	ItemCount int64
	CreatedAt time.Time
}

// CollectionRepository is the persistence port for wishlist collections.
type CollectionRepository interface {
	CreateCollection(ctx context.Context, userID, name string) (Collection, error)
	// ListCollections returns the user's collections with item_count populated.
	ListCollections(ctx context.Context, userID string) ([]Collection, error)
	// AddToCollection is idempotent; added=false if the pair already existed.
	AddToCollection(ctx context.Context, userID, collectionID, listingID string) (added bool, err error)
	// RemoveFromCollection is idempotent; removed=false if it wasn't there.
	RemoveFromCollection(ctx context.Context, userID, collectionID, listingID string) (removed bool, err error)
	// ListCollectionItems returns listing ids in the collection, keyset by id.
	ListCollectionItems(ctx context.Context, userID, collectionID, cursor string, pageSize int32) (ids []string, next string, total int64, err error)
}

// ── Postgres Implementation ──────────────────────────────────────────

type PostgresCollectionRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresCollectionRepository(pool *pgxpool.Pool) *PostgresCollectionRepository {
	return &PostgresCollectionRepository{pool: pool}
}

func (r *PostgresCollectionRepository) CreateCollection(ctx context.Context, userID, name string) (Collection, error) {
	c := Collection{ID: uuid.NewString(), UserID: userID, Name: name, CreatedAt: time.Now()}
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO collections (id, user_id, name, created_at) VALUES ($1, $2, $3, $4)`,
		c.ID, c.UserID, c.Name, c.CreatedAt); err != nil {
		return Collection{}, fmt.Errorf("insert collection: %w", err)
	}
	return c, nil
}

func (r *PostgresCollectionRepository) ListCollections(ctx context.Context, userID string) ([]Collection, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT c.id, c.user_id, c.name, c.created_at, COUNT(ci.listing_id)
		 FROM collections c
		 LEFT JOIN collection_items ci ON ci.collection_id = c.id
		 WHERE c.user_id = $1
		 GROUP BY c.id, c.user_id, c.name, c.created_at
		 ORDER BY c.created_at DESC`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	defer rows.Close()

	var out []Collection
	for rows.Next() {
		var c Collection
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.CreatedAt, &c.ItemCount); err != nil {
			return nil, fmt.Errorf("scan collection: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ownsCollection returns ErrCollectionNotFound if the collection is absent or
// owned by a different user.
func (r *PostgresCollectionRepository) ownsCollection(ctx context.Context, userID, collectionID string) error {
	var owner string
	err := r.pool.QueryRow(ctx, `SELECT user_id FROM collections WHERE id = $1`, collectionID).Scan(&owner)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCollectionNotFound
		}
		return fmt.Errorf("load collection: %w", err)
	}
	if owner != userID {
		return ErrCollectionNotFound
	}
	return nil
}

func (r *PostgresCollectionRepository) AddToCollection(ctx context.Context, userID, collectionID, listingID string) (bool, error) {
	if err := r.ownsCollection(ctx, userID, collectionID); err != nil {
		return false, err
	}
	tag, err := r.pool.Exec(ctx,
		`INSERT INTO collection_items (collection_id, listing_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		collectionID, listingID)
	if err != nil {
		return false, fmt.Errorf("add collection item: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *PostgresCollectionRepository) RemoveFromCollection(ctx context.Context, userID, collectionID, listingID string) (bool, error) {
	if err := r.ownsCollection(ctx, userID, collectionID); err != nil {
		return false, err
	}
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM collection_items WHERE collection_id = $1 AND listing_id = $2`,
		collectionID, listingID)
	if err != nil {
		return false, fmt.Errorf("remove collection item: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *PostgresCollectionRepository) ListCollectionItems(ctx context.Context, userID, collectionID, cursor string, pageSize int32) ([]string, string, int64, error) {
	if err := r.ownsCollection(ctx, userID, collectionID); err != nil {
		return nil, "", 0, err
	}
	limit := clampPageSize(pageSize)
	rows, err := r.pool.Query(ctx,
		`SELECT listing_id FROM collection_items
		 WHERE collection_id = $1 AND ($2 = '' OR listing_id > $2)
		 ORDER BY listing_id LIMIT $3`,
		collectionID, cursor, limit+1)
	if err != nil {
		return nil, "", 0, fmt.Errorf("list collection items: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, "", 0, fmt.Errorf("scan collection item: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, "", 0, err
	}

	var next string
	if len(ids) > limit {
		ids = ids[:limit]
		next = ids[len(ids)-1]
	}

	var total int64
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM collection_items WHERE collection_id = $1`, collectionID).Scan(&total); err != nil {
		return nil, "", 0, fmt.Errorf("count collection items: %w", err)
	}
	return ids, next, total, nil
}

var _ CollectionRepository = (*PostgresCollectionRepository)(nil)

// ── InMemory Implementation ───────────────────────────────────────────

type InMemoryCollectionRepository struct {
	mu    sync.Mutex
	cols  map[string]Collection          // collectionID -> collection
	items map[string]map[string]struct{} // collectionID -> set(listingID)
}

func NewInMemoryCollectionRepository() *InMemoryCollectionRepository {
	return &InMemoryCollectionRepository{
		cols:  map[string]Collection{},
		items: map[string]map[string]struct{}{},
	}
}

func (r *InMemoryCollectionRepository) CreateCollection(_ context.Context, userID, name string) (Collection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := Collection{ID: uuid.NewString(), UserID: userID, Name: name, CreatedAt: time.Now()}
	r.cols[c.ID] = c
	r.items[c.ID] = map[string]struct{}{}
	return c, nil
}

func (r *InMemoryCollectionRepository) ListCollections(_ context.Context, userID string) ([]Collection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Collection
	for id, c := range r.cols {
		if c.UserID != userID {
			continue
		}
		c.ItemCount = int64(len(r.items[id]))
		out = append(out, c)
	}
	// newest first
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (r *InMemoryCollectionRepository) owns(userID, collectionID string) error {
	c, ok := r.cols[collectionID]
	if !ok || c.UserID != userID {
		return ErrCollectionNotFound
	}
	return nil
}

func (r *InMemoryCollectionRepository) AddToCollection(_ context.Context, userID, collectionID, listingID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.owns(userID, collectionID); err != nil {
		return false, err
	}
	set := r.items[collectionID]
	if _, ok := set[listingID]; ok {
		return false, nil
	}
	set[listingID] = struct{}{}
	return true, nil
}

func (r *InMemoryCollectionRepository) RemoveFromCollection(_ context.Context, userID, collectionID, listingID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.owns(userID, collectionID); err != nil {
		return false, err
	}
	set := r.items[collectionID]
	if _, ok := set[listingID]; !ok {
		return false, nil
	}
	delete(set, listingID)
	return true, nil
}

func (r *InMemoryCollectionRepository) ListCollectionItems(_ context.Context, userID, collectionID, cursor string, pageSize int32) ([]string, string, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.owns(userID, collectionID); err != nil {
		return nil, "", 0, err
	}
	all := make([]string, 0, len(r.items[collectionID]))
	for id := range r.items[collectionID] {
		all = append(all, id)
	}
	sort.Strings(all)

	limit := clampPageSize(pageSize)
	ids := make([]string, 0, limit)
	var next string
	for _, id := range all {
		if cursor != "" && id <= cursor {
			continue
		}
		if len(ids) == limit {
			next = ids[len(ids)-1]
			break
		}
		ids = append(ids, id)
	}
	return ids, next, int64(len(all)), nil
}

var _ CollectionRepository = (*InMemoryCollectionRepository)(nil)
