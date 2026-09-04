package service

import (
	"context"
	"errors"

	"github.com/buidangphuc/team-domain/internal/repository"
)

// ErrSlugRequired is returned when an upsert omits the slug. The handler maps it
// to codes.InvalidArgument.
var ErrSlugRequired = errors.New("slug is required")

// StorefrontService holds the storefront business logic (F5), independent of
// gRPC and of storage. It mirrors ListingService: depend on a repository port,
// return domain values, let the handler adapt to protobuf.
type StorefrontService struct {
	repo repository.StorefrontRepository
}

func NewStorefrontService(repo repository.StorefrontRepository) *StorefrontService {
	return &StorefrontService{repo: repo}
}

// Upsert creates or replaces the calling seller's storefront. It is
// owner-scoped: the stored seller_id is always the authenticated ownerID, so a
// seller can never write another seller's row regardless of request contents.
func (s *StorefrontService) Upsert(ctx context.Context, sf repository.Storefront, ownerID string) (repository.Storefront, error) {
	if ownerID == "" {
		return repository.Storefront{}, ErrForbidden
	}
	if sf.Slug == "" {
		return repository.Storefront{}, ErrSlugRequired
	}
	sf.SellerID = ownerID // owner is server-forced, never client-supplied
	return s.repo.Upsert(ctx, sf)
}

// Get resolves a storefront by seller_id, falling back to slug when seller_id is
// empty. A lookup with neither key set returns ErrStorefrontNotFound.
func (s *StorefrontService) Get(ctx context.Context, sellerID, slug string) (repository.Storefront, error) {
	if sellerID != "" {
		return s.repo.GetBySeller(ctx, sellerID)
	}
	if slug != "" {
		return s.repo.GetBySlug(ctx, slug)
	}
	return repository.Storefront{}, repository.ErrStorefrontNotFound
}
