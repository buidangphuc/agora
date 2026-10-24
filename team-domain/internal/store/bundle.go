package store

import (
	"context"
	"errors"
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
