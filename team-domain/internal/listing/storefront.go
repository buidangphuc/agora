package listing

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/buidangphuc/team-domain/internal/store"
)

// ErrSlugRequired is returned when an upsert omits the slug. The handler maps it
// to codes.InvalidArgument.
var ErrSlugRequired = errors.New("slug is required")

// ErrDisplayNameInvalid is returned when display_name is longer than
// MaxDisplayNameRunes or contains control characters. Mapped to InvalidArgument.
var ErrDisplayNameInvalid = errors.New("display_name must be at most 80 characters with no control characters")

// ErrTooManySellerIDs is returned by GetMany when more than MaxBatchSellerIDs
// distinct ids are requested. Mapped to InvalidArgument.
var ErrTooManySellerIDs = errors.New("too many seller_ids (max 100)")

const (
	// MaxDisplayNameRunes caps the shop display name after trimming.
	MaxDisplayNameRunes = 80
	// MaxBatchSellerIDs caps BatchGetStorefronts.
	MaxBatchSellerIDs = 100
)

// StorefrontService holds the storefront business logic (F5), independent of
// gRPC and of storage. It mirrors Service: depend on a repository port,
// return domain values, let the handler adapt to protobuf.
type StorefrontService struct {
	repo store.StorefrontRepository
}

// NewStorefrontService builds the storefront service over its repository.
func NewStorefrontService(repo store.StorefrontRepository) *StorefrontService {
	return &StorefrontService{repo: repo}
}

// Upsert creates or replaces the calling seller's storefront. It is
// owner-scoped: the stored seller_id is always the authenticated ownerID, so a
// seller can never write another seller's row regardless of request contents.
func (s *StorefrontService) Upsert(ctx context.Context, sf store.Storefront, ownerID string) (store.Storefront, error) {
	if ownerID == "" {
		return store.Storefront{}, ErrForbidden
	}
	if sf.Slug == "" {
		return store.Storefront{}, ErrSlugRequired
	}
	name := strings.TrimSpace(sf.DisplayName)
	if utf8.RuneCountInString(name) > MaxDisplayNameRunes || strings.ContainsFunc(name, unicode.IsControl) {
		return store.Storefront{}, ErrDisplayNameInvalid
	}
	sf.DisplayName = name
	sf.SellerID = ownerID // owner is server-forced, never client-supplied
	return s.repo.Upsert(ctx, sf)
}

// Get resolves a storefront by seller_id, falling back to slug when seller_id is
// empty. A lookup with neither key set returns ErrStorefrontNotFound.
func (s *StorefrontService) Get(ctx context.Context, sellerID, slug string) (store.Storefront, error) {
	if sellerID != "" {
		return s.repo.GetBySeller(ctx, sellerID)
	}
	if slug != "" {
		return s.repo.GetBySlug(ctx, slug)
	}
	return store.Storefront{}, store.ErrStorefrontNotFound
}

// GetMany resolves storefronts for a batch of seller ids with one store query.
// Empty ids and duplicates are dropped; more than MaxBatchSellerIDs distinct ids
// is ErrTooManySellerIDs; unknown sellers are omitted from the result.
func (s *StorefrontService) GetMany(ctx context.Context, sellerIDs []string) ([]store.Storefront, error) {
	seen := make(map[string]struct{}, len(sellerIDs))
	ids := make([]string, 0, len(sellerIDs))
	for _, id := range sellerIDs {
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) > MaxBatchSellerIDs {
		return nil, ErrTooManySellerIDs
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return s.repo.GetBySellers(ctx, ids)
}
