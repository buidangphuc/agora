package store

import (
	"context"
	"errors"
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
	// DisplayName is the seller-chosen public shop name (trimmed, <=80 runes).
	// Empty means unset. Stored inside the JSONB config (no migration).
	DisplayName string
	UpdatedAt   time.Time
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
	// GetBySellers loads the storefronts of the given seller ids in one query.
	// Sellers without a storefront are omitted; order is unspecified.
	GetBySellers(ctx context.Context, sellerIDs []string) ([]Storefront, error)
}
