package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/buidangphuc/team-promotion/internal/repository"
)

// ErrInvalidAdCampaign is returned for malformed CreateAdCampaign input.
var ErrInvalidAdCampaign = errors.New("invalid ad campaign")

// SponsoredService holds the sponsored-placement / ad-campaign business logic.
// Campaigns are a MOCK ledger: CreateAdCampaign records the chosen budget/bid but
// never moves money (payments/wallet stay mock, AGENTS.md §7).
type SponsoredService struct {
	repo   repository.AdCampaignRepository
	logger *slog.Logger
}

// NewSponsoredService wires the ad-campaign repository.
func NewSponsoredService(repo repository.AdCampaignRepository, logger *slog.Logger) *SponsoredService {
	if logger == nil {
		logger = slog.Default()
	}
	return &SponsoredService{repo: repo, logger: logger}
}

// CreateAdCampaignParams is the validated input for CreateAdCampaign. SellerID is
// bound by the caller from the authenticated seller principal — the wire request
// carries no seller_id.
type CreateAdCampaignParams struct {
	SellerID  string
	ListingID string
	Budget    int64
	Bid       int64
}

// CreateAdCampaign records a sponsored campaign for a seller (MOCK — no charge).
// It validates the listing id and non-negative budget/bid, then persists an active
// campaign. Returns the stored campaign.
func (s *SponsoredService) CreateAdCampaign(ctx context.Context, p CreateAdCampaignParams) (repository.AdCampaign, error) {
	if strings.TrimSpace(p.SellerID) == "" {
		return repository.AdCampaign{}, ErrInvalidAdCampaign
	}
	if strings.TrimSpace(p.ListingID) == "" {
		return repository.AdCampaign{}, ErrInvalidAdCampaign
	}
	if p.Budget < 0 || p.Bid < 0 {
		return repository.AdCampaign{}, ErrInvalidAdCampaign
	}
	created, err := s.repo.Create(ctx, repository.AdCampaign{
		SellerID:  strings.TrimSpace(p.SellerID),
		ListingID: strings.TrimSpace(p.ListingID),
		Budget:    p.Budget,
		Bid:       p.Bid,
		Status:    repository.AdCampaignStatusActive,
	})
	if err != nil {
		return repository.AdCampaign{}, err
	}
	return created, nil
}

// ListSponsoredSlots returns the listing ids to render in sponsored slots for a
// placement context, ordered best-bid first. The context key is accepted for
// forward compatibility (real placement targeting is out of scope for the mock);
// today it returns every active campaign's listing, deduplicated, keeping the
// highest-bid entry per listing. An empty/anonymous context never panics.
func (s *SponsoredService) ListSponsoredSlots(ctx context.Context, contextStr string) ([]string, error) {
	campaigns, err := s.repo.ListActive(ctx, defaultPageSize)
	if err != nil {
		return nil, err
	}
	// campaigns arrive bid-descending from the repository. Dedup by listing_id,
	// keeping the first (highest-bid) occurrence so a listing appears once.
	seen := make(map[string]struct{}, len(campaigns))
	out := make([]string, 0, len(campaigns))
	for _, c := range campaigns {
		if c.ListingID == "" {
			continue
		}
		if _, dup := seen[c.ListingID]; dup {
			continue
		}
		seen[c.ListingID] = struct{}{}
		out = append(out, c.ListingID)
	}
	return out, nil
}
