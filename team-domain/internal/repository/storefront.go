package repository

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Storefront errors. ErrStorefrontNotFound is returned by the Get* methods when
// no row matches; ErrSlugTaken is returned by Upsert when another seller already
// owns the requested slug (the DB unique index on slug is the source of truth).
var (
	ErrStorefrontNotFound = errors.New("storefront not found")
	ErrSlugTaken          = errors.New("storefront slug already taken")
)

// Storefront is the persistence-layer domain value for a seller's public shop
// page (F5). The presentational fields (banner/tagline/featured/theme) are
// stored together as a JSONB `config` blob in Postgres; seller_id and slug are
// promoted to their own columns (primary key / unique index).
type Storefront struct {
	SellerID           string
	Slug               string
	BannerURL          string
	Tagline            string
	FeaturedListingIDs []string
	Theme              string
	UpdatedAt          time.Time
}

// StorefrontRepository is the port the storefront service depends on. Upsert is
// keyed on SellerID (a seller has at most one storefront); reads resolve by
// seller_id OR slug.
type StorefrontRepository interface {
	// Upsert inserts or replaces the storefront keyed by s.SellerID. It returns
	// ErrSlugTaken if s.Slug is already owned by a different seller.
	Upsert(ctx context.Context, s Storefront) (Storefront, error)
	// GetBySeller loads a storefront by owner id, or ErrStorefrontNotFound.
	GetBySeller(ctx context.Context, sellerID string) (Storefront, error)
	// GetBySlug loads a storefront by its public slug, or ErrStorefrontNotFound.
	GetBySlug(ctx context.Context, slug string) (Storefront, error)
}

// InMemoryStorefrontRepository is a deterministic fake store for tests. It
// enforces the same slug-uniqueness invariant as the Postgres unique index so
// unit tests exercise the cross-seller conflict path without a database.
type InMemoryStorefrontRepository struct {
	mu       sync.RWMutex
	bySeller map[string]Storefront
}

// NewInMemoryStorefrontRepository returns an empty fake store.
func NewInMemoryStorefrontRepository(seed ...Storefront) *InMemoryStorefrontRepository {
	bySeller := make(map[string]Storefront, len(seed))
	for _, s := range seed {
		bySeller[s.SellerID] = s
	}
	return &InMemoryStorefrontRepository{bySeller: bySeller}
}

// Upsert inserts or replaces the seller's storefront, rejecting a slug already
// claimed by another seller.
func (r *InMemoryStorefrontRepository) Upsert(_ context.Context, s Storefront) (Storefront, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Slug-uniqueness guard across sellers (mirrors storefronts_slug_key).
	for sellerID, existing := range r.bySeller {
		if existing.Slug == s.Slug && sellerID != s.SellerID {
			return Storefront{}, ErrSlugTaken
		}
	}
	s.UpdatedAt = time.Now().UTC()
	r.bySeller[s.SellerID] = s
	return s, nil
}

// GetBySeller returns the seller's storefront or ErrStorefrontNotFound.
func (r *InMemoryStorefrontRepository) GetBySeller(_ context.Context, sellerID string) (Storefront, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.bySeller[sellerID]
	if !ok {
		return Storefront{}, ErrStorefrontNotFound
	}
	return s, nil
}

// GetBySlug returns the storefront with the given slug or ErrStorefrontNotFound.
func (r *InMemoryStorefrontRepository) GetBySlug(_ context.Context, slug string) (Storefront, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.bySeller {
		if s.Slug == slug {
			return s, nil
		}
	}
	return Storefront{}, ErrStorefrontNotFound
}

// compile-time assertion that the fake satisfies the port.
var _ StorefrontRepository = (*InMemoryStorefrontRepository)(nil)
