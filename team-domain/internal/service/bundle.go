package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-domain/internal/repository"
)

// ErrTitleRequired is returned when a bundle create omits the title. The handler
// maps it to codes.InvalidArgument.
var ErrTitleRequired = errors.New("title is required")

// BundleService holds the bundle business logic (F5), independent of gRPC and of
// storage. It mirrors ListingService/StorefrontService: depend on a repository
// port, return domain values, let the handler adapt to protobuf.
type BundleService struct {
	repo repository.BundleRepository
}

func NewBundleService(repo repository.BundleRepository) *BundleService {
	return &BundleService{repo: repo}
}

// Create stores a new bundle for the calling seller. It is owner-scoped: the
// stored seller_id is always the authenticated ownerID, so a seller can never
// create a bundle attributed to another seller regardless of request contents.
// The id is server-assigned. An anonymous caller (empty ownerID) is rejected.
func (s *BundleService) Create(ctx context.Context, b repository.Bundle, ownerID string) (repository.Bundle, error) {
	if ownerID == "" {
		return repository.Bundle{}, ErrForbidden
	}
	if b.Title == "" {
		return repository.Bundle{}, ErrTitleRequired
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
func (s *BundleService) Get(ctx context.Context, id string) (repository.Bundle, error) {
	return s.repo.Get(ctx, id)
}

// ListBySeller returns a keyset page of the seller's bundles.
func (s *BundleService) ListBySeller(ctx context.Context, sellerID, cursor string, pageSize int32) (repository.BundlePage, error) {
	return s.repo.ListBySeller(ctx, sellerID, cursor, pageSize)
}
