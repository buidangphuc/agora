package repository

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// ErrBundleNotFound is returned by the bundle Get when no row matches.
var ErrBundleNotFound = errors.New("bundle not found")

// Bundle is the persistence-layer domain value for a seller-defined bundle of
// listings (F5). It is decoupled from the generated protobuf Bundle: the handler
// maps between wire and domain so the contract and storage model evolve
// independently. Mirrors the Listing domain value.
type Bundle struct {
	ID          string
	SellerID    string // owner Principal id (server-assigned on create)
	Title       string
	ListingIDs  []string // member listing ids
	BundlePrice int64    // minor units (e.g. VND), integer to avoid float money
	CreatedAt   time.Time
}

// BundlePage is a single page of bundles plus keyset-pagination metadata,
// mirroring Page for listings.
type BundlePage struct {
	Items      []Bundle
	NextCursor string // empty => no further pages
	Total      int64  // total rows matching the query
}

// BundleRepository is the port the bundle service depends on. Cursor is an
// opaque keyset (the last id of the previous page); empty means "from the start".
type BundleRepository interface {
	// Create inserts a new bundle (id + seller_id must be set by the caller).
	Create(ctx context.Context, b Bundle) (Bundle, error)
	// Get returns the bundle for id, or ErrBundleNotFound.
	Get(ctx context.Context, id string) (Bundle, error)
	// ListBySeller returns a keyset page of the seller's bundles.
	ListBySeller(ctx context.Context, sellerID, cursor string, pageSize int32) (BundlePage, error)
}

// InMemoryBundleRepository is a deterministic fake store for tests. Production
// uses PostgresBundleRepository (bundle_pg.go).
type InMemoryBundleRepository struct {
	mu   sync.RWMutex
	byID map[string]Bundle
}

// NewInMemoryBundleRepository returns a store seeded with the given rows (empty
// by default).
func NewInMemoryBundleRepository(seed ...Bundle) *InMemoryBundleRepository {
	byID := make(map[string]Bundle, len(seed))
	for _, b := range seed {
		byID[b.ID] = b
	}
	return &InMemoryBundleRepository{byID: byID}
}

// Create inserts a new bundle (overwrites on duplicate id in this fake).
func (r *InMemoryBundleRepository) Create(_ context.Context, b Bundle) (Bundle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b.CreatedAt.IsZero() {
		b.CreatedAt = time.Now().UTC()
	}
	r.byID[b.ID] = b
	return b, nil
}

// Get returns the bundle for id, or ErrBundleNotFound.
func (r *InMemoryBundleRepository) Get(_ context.Context, id string) (Bundle, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.byID[id]
	if !ok {
		return Bundle{}, ErrBundleNotFound
	}
	return b, nil
}

// ListBySeller returns a keyset page of the seller's bundles ordered by id.
func (r *InMemoryBundleRepository) ListBySeller(_ context.Context, sellerID, cursor string, pageSize int32) (BundlePage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.byID))
	var total int64
	for id, b := range r.byID {
		if b.SellerID == sellerID {
			ids = append(ids, id)
			total++
		}
	}
	sort.Strings(ids)

	limit := clampPageSize(pageSize)
	items := make([]Bundle, 0, limit)
	var next string
	for _, id := range ids {
		if cursor != "" && id <= cursor {
			continue
		}
		if len(items) == limit {
			next = items[len(items)-1].ID
			break
		}
		items = append(items, r.byID[id])
	}
	return BundlePage{Items: items, NextCursor: next, Total: total}, nil
}

// compile-time assertion that the fake satisfies the port.
var _ BundleRepository = (*InMemoryBundleRepository)(nil)
