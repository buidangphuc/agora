// Package repository is the persistence boundary. It exposes domain types and
// an interface; the concrete store is swappable (in-memory here, Postgres in a
// real service). Nothing above this package imports a DB driver directly.
package repository

import (
	"context"
	"errors"
	"sync"
)

// ErrNotFound is returned when a lookup misses. The service layer maps this to
// a codes.NotFound / platform.common.v1.Error{code:"not_found"} at the edge.
var ErrNotFound = errors.New("listing not found")

// Listing is the persistence-layer domain value. It is intentionally decoupled
// from the generated protobuf Listing: the handler maps between wire and domain
// so the contract and the storage model can evolve independently.
type Listing struct {
	ID          string
	Title       string
	Description string
	Price       int64  // minor units (e.g. VND), integer to avoid float money
	Currency    string // ISO 4217
	Status      string // "draft" | "published" | "rejected"
}

// ListingRepository is the port the service depends on.
type ListingRepository interface {
	Get(ctx context.Context, id string) (Listing, error)
	List(ctx context.Context, cursor string, pageSize int32) (items []Listing, nextCursor string, err error)
}

// InMemoryListingRepository is a stub store for the template. Swap it for a
// Postgres-backed impl in a real service.
//
// Real impl sketch (DATABASE_URL, e.g.
//   postgresql://listing_svc:listing_pass@localhost:5433/listing_db):
//
//	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
//	// then implement Get/List with SQL against the `listings` table
//	// created by migrations/0001_init.sql.
//
// DB-per-service (ADR/arch): ONLY this service connects to listing_db. Other
// services reach listing data through gRPC, never by sharing the database.
type InMemoryListingRepository struct {
	mu   sync.RWMutex
	byID map[string]Listing
}

// NewInMemoryListingRepository returns a store seeded with one row so the seed
// RPC has something to return.
func NewInMemoryListingRepository() *InMemoryListingRepository {
	return &InMemoryListingRepository{
		byID: map[string]Listing{
			"seed-1": {
				ID:          "seed-1",
				Title:       "Seed listing",
				Description: "Hardcoded row proving the read path end-to-end.",
				Price:       1_500_000_000,
				Currency:    "VND",
				Status:      "published",
			},
		},
	}
}

// Get returns the listing for id, or ErrNotFound.
func (r *InMemoryListingRepository) Get(_ context.Context, id string) (Listing, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	l, ok := r.byID[id]
	if !ok {
		return Listing{}, ErrNotFound
	}
	return l, nil
}

// List returns all listings in one page. A real impl paginates via the opaque
// cursor and clamps pageSize to a server max.
func (r *InMemoryListingRepository) List(_ context.Context, _ string, _ int32) ([]Listing, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]Listing, 0, len(r.byID))
	for _, l := range r.byID {
		items = append(items, l)
	}
	return items, "", nil // empty nextCursor => no further pages
}

// compile-time assertion that the stub satisfies the port.
var _ ListingRepository = (*InMemoryListingRepository)(nil)
