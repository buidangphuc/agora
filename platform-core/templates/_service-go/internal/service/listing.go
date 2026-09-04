// Package service holds business logic, independent of gRPC and of storage.
// It depends on the repository port and returns domain values; the handler
// (internal/handler) adapts these to the generated protobuf types.
package service

import (
	"context"

	"github.com/your-org/team-service/internal/repository"
)

// ListingService is the business-logic placeholder. Real rules
// (authorization beyond scopes, validation, orchestration) live here.
type ListingService struct {
	repo repository.ListingRepository
}

// NewListingService wires the service to a repository implementation.
func NewListingService(repo repository.ListingRepository) *ListingService {
	return &ListingService{repo: repo}
}

// Get returns a single listing by id. It passes repository.ErrNotFound through;
// the handler maps that to a gRPC NotFound status.
func (s *ListingService) Get(ctx context.Context, id string) (repository.Listing, error) {
	return s.repo.Get(ctx, id)
}

// List returns a page of listings.
func (s *ListingService) List(ctx context.Context, cursor string, pageSize int32) ([]repository.Listing, string, error) {
	return s.repo.List(ctx, cursor, pageSize)
}
