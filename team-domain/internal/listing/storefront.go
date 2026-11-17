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

var ErrTaglineInvalid = errors.New("tagline must be at most 120 characters with no control characters")

var ErrBannerInvalid = errors.New("banner must be an uploaded image URL")

var ErrThemeInvalid = errors.New("theme must be default, dark or festive")

var ErrFeaturedInvalid = errors.New("featured listing ids are invalid")

var ErrTooManyFeatured = errors.New("too many featured listings (max 6)")

const (
	MaxTaglineRunes = 120
	MaxFeatured     = 6
	MaxBannerBytes  = 2048
	// MaxDisplayNameRunes caps the shop display name after trimming.
	MaxDisplayNameRunes = 80
	// MaxBatchSellerIDs caps BatchGetStorefronts.
	MaxBatchSellerIDs = 100
)

// StorefrontService holds the storefront business logic (F5), independent of
// gRPC and of storage. It mirrors ListingService: depend on a repository port,
// return domain values, let the handler adapt to protobuf.
type StorefrontService struct {
	repo       store.StorefrontRepository
	listings   store.ListingRepository
	publicBase string
}

func NewStorefrontService(repo store.StorefrontRepository, publicBase string) *StorefrontService {
	return &StorefrontService{repo: repo, publicBase: strings.TrimRight(publicBase, "/")}
}

func (s *StorefrontService) bannerAllowed(banner string) bool {
	if len(banner) > MaxBannerBytes || strings.ContainsAny(banner, "?#") {
		return false
	}
	rest, ok := strings.CutPrefix(banner, s.publicBase+"/")
	return ok && rest != ""
}

func validTheme(t string) bool {
	switch t {
	case "", "default", "dark", "festive":
		return true
	}
	return false
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
	tagline := strings.TrimSpace(sf.Tagline)
	if utf8.RuneCountInString(tagline) > MaxTaglineRunes || strings.ContainsFunc(tagline, unicode.IsControl) {
		return store.Storefront{}, ErrTaglineInvalid
	}
	sf.Tagline = tagline
	if sf.BannerURL != "" && !s.bannerAllowed(sf.BannerURL) {
		existing, err := s.repo.GetBySeller(ctx, ownerID)
		if err != nil && !errors.Is(err, store.ErrStorefrontNotFound) {
			return store.Storefront{}, err
		}
		if err != nil || existing.BannerURL != sf.BannerURL {
			return store.Storefront{}, ErrBannerInvalid
		}
	}
	if !validTheme(sf.Theme) {
		return store.Storefront{}, ErrThemeInvalid
	}
	if err := s.validateFeatured(ctx, sf.FeaturedListingIDs, ownerID); err != nil {
		return store.Storefront{}, err
	}
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

func (s *StorefrontService) WithListings(repo store.ListingRepository) *StorefrontService {
	s.listings = repo
	return s
}

func (s *StorefrontService) validateFeatured(ctx context.Context, ids []string, ownerID string) error {
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > MaxFeatured {
		return ErrTooManyFeatured
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup || id == "" {
			return ErrFeaturedInvalid
		}
		seen[id] = struct{}{}
	}
	if s.listings == nil {
		return nil
	}
	found, err := s.listings.GetByIDs(ctx, ids)
	if err != nil {
		return err
	}
	owned := make(map[string]struct{}, len(found))
	for _, l := range found {
		if l.SellerID == ownerID {
			owned[l.ID] = struct{}{}
		}
	}
	for _, id := range ids {
		if _, ok := owned[id]; !ok {
			return ErrFeaturedInvalid
		}
	}
	return nil
}
