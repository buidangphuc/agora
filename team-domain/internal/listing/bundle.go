package listing

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-domain/internal/store"
)

// ErrTitleRequired is returned when a bundle create omits the title. The handler
// maps it to codes.InvalidArgument.
var ErrTitleRequired = errors.New("title is required")

// BundleService holds the bundle business logic (F5), independent of gRPC and of
// storage. It mirrors ListingService/StorefrontService: depend on a repository
// port, return domain values, let the handler adapt to protobuf.
type BundleService struct {
	repo     store.BundleRepository
	listings store.ListingRepository
}

// STUB (test-author): signature and sentinels only; the implementer replaces
// the behaviour (task 1.2).
var (
	ErrBundleListingCount     = errors.New("a bundle needs 2 to 10 listings")
	ErrBundleDuplicateListing = errors.New("a bundle cannot repeat a listing")
	ErrBundleListingNotOwned  = errors.New("listing not found or not yours")
	ErrBundleVariantListing   = errors.New("a listing with variants cannot be bundled")
	ErrBundlePrice            = errors.New("bundle price must be above 0 and below the sum of the listings")
)

func NewBundleService(repo store.BundleRepository, listings store.ListingRepository) *BundleService {
	return &BundleService{repo: repo, listings: listings}
}

// Create stores a new bundle for the calling seller. It is owner-scoped: the
// stored seller_id is always the authenticated ownerID, so a seller can never
// create a bundle attributed to another seller regardless of request contents.
// The id is server-assigned. An anonymous caller (empty ownerID) is rejected.
func (s *BundleService) Create(ctx context.Context, b store.Bundle, ownerID string) (store.Bundle, error) {
	if ownerID == "" {
		return store.Bundle{}, ErrForbidden
	}
	if b.Title == "" {
		return store.Bundle{}, ErrTitleRequired
	}
	b.SellerID = ownerID // owner is server-forced, never client-supplied
	if b.ID == "" {
		b.ID = uuid.NewString()
	}
	if b.ListingIDs == nil {
		b.ListingIDs = []string{}
	}
	return s.repo.Create(ctx, b)
}

// Get resolves a bundle by id, or ErrBundleNotFound.
func (s *BundleService) Get(ctx context.Context, id string) (store.Bundle, error) {
	return s.repo.Get(ctx, id)
}

// ListBySeller returns a keyset page of the seller's bundles.
func (s *BundleService) ListBySeller(ctx context.Context, sellerID, cursor string, pageSize int32) (store.BundlePage, error) {
	return s.repo.ListBySeller(ctx, sellerID, cursor, pageSize)
}
