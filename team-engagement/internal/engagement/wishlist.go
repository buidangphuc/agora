package engagement

import (
	"context"
	"errors"
	"log/slog"

	"github.com/buidangphuc/team-engagement/internal/store"
)

var (
	ErrEmptyCollectionName = errors.New("collection name is required")
	ErrEmptyCollectionID   = errors.New("collection_id is required")
)

// CollectionService owns wishlist-collection use cases (F3): named per-user
// lists with idempotent add/remove of listing items.
type CollectionService struct {
	repo   store.CollectionRepository
	logger *slog.Logger
}

func NewCollectionService(repo store.CollectionRepository, logger *slog.Logger) *CollectionService {
	if logger == nil {
		logger = slog.Default()
	}
	return &CollectionService{repo: repo, logger: logger}
}

func (s *CollectionService) CreateCollection(ctx context.Context, userID, name string) (store.Collection, error) {
	if name == "" {
		return store.Collection{}, ErrEmptyCollectionName
	}
	return s.repo.CreateCollection(ctx, userID, name)
}

func (s *CollectionService) ListCollections(ctx context.Context, userID string) ([]store.Collection, error) {
	return s.repo.ListCollections(ctx, userID)
}

func (s *CollectionService) AddToCollection(ctx context.Context, userID, collectionID, listingID string) (bool, error) {
	if collectionID == "" {
		return false, ErrEmptyCollectionID
	}
	if listingID == "" {
		return false, ErrEmptyListing
	}
	return s.repo.AddToCollection(ctx, userID, collectionID, listingID)
}

func (s *CollectionService) RemoveFromCollection(ctx context.Context, userID, collectionID, listingID string) (bool, error) {
	if collectionID == "" {
		return false, ErrEmptyCollectionID
	}
	if listingID == "" {
		return false, ErrEmptyListing
	}
	return s.repo.RemoveFromCollection(ctx, userID, collectionID, listingID)
}

func (s *CollectionService) ListCollectionItems(ctx context.Context, userID, collectionID, cursor string, pageSize int32) ([]string, string, int64, error) {
	if collectionID == "" {
		return nil, "", 0, ErrEmptyCollectionID
	}
	return s.repo.ListCollectionItems(ctx, userID, collectionID, cursor, pageSize)
}
