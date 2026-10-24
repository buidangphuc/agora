package sponsored

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/team-promotion/internal/listing"
	"github.com/buidangphuc/team-promotion/internal/store"
)

const defaultPageSize = 50

var ErrInvalidAdCampaign = errors.New("invalid ad campaign")

type Service struct {
	repo      store.AdCampaignRepository
	listings  listing.Getter
	audit     *auditx.Gate
	logger    *slog.Logger
	maxBid    int64
	maxBudget int64
}

// Default ceilings (minor units) used when none are configured; they match the
// MAX_AD_BID / MAX_AD_BUDGET config defaults.
const (
	DefaultMaxAdBid    int64 = 1_000_000
	DefaultMaxAdBudget int64 = 1_000_000_000
)

func New(repo store.AdCampaignRepository, listings listing.Getter, audit *auditx.Gate, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: repo, listings: listings, audit: audit, logger: logger, maxBid: DefaultMaxAdBid, maxBudget: DefaultMaxAdBudget}
}

// WithLimits sets the bid and budget ceilings. A non-positive value keeps the
// default, so a zero-value config can never mean "unbounded".
func (s *Service) WithLimits(maxBid, maxBudget int64) *Service {
	if maxBid > 0 {
		s.maxBid = maxBid
	}
	if maxBudget > 0 {
		s.maxBudget = maxBudget
	}
	return s
}

// CreateAdCampaignParams is the validated input for CreateAdCampaign. SellerID is
// bound by the caller from the authenticated seller principal — the wire request
// carries no seller_id.
type CreateAdCampaignParams struct {
	SellerID  string
	ListingID string
	Budget    int64
	Bid       int64

	// IsAdmin skips the listing-ownership check.
	IsAdmin bool
}

// CreateAdCampaign records a sponsored campaign for a seller (MOCK — no charge).
// It validates the listing id and non-negative budget/bid within the configured
// maxima, requires non-admin callers to own the listing, then persists an active
// campaign. Ownership errors are gRPC status errors (PermissionDenied, Unavailable).
func (s *Service) CreateAdCampaign(ctx context.Context, p CreateAdCampaignParams) (store.AdCampaign, error) {
	if strings.TrimSpace(p.SellerID) == "" {
		return store.AdCampaign{}, ErrInvalidAdCampaign
	}
	if strings.TrimSpace(p.ListingID) == "" {
		return store.AdCampaign{}, ErrInvalidAdCampaign
	}
	if p.Budget < 0 || p.Bid < 0 || p.Budget > s.maxBudget || p.Bid > s.maxBid {
		return store.AdCampaign{}, ErrInvalidAdCampaign
	}
	listingID := strings.TrimSpace(p.ListingID)
	sellerID := strings.TrimSpace(p.SellerID)
	c := store.AdCampaign{
		SellerID:  sellerID,
		ListingID: listingID,
		Budget:    p.Budget,
		Bid:       p.Bid,
		Status:    store.AdCampaignStatusActive,
	}
	var created store.AdCampaign
	insert := func(ctx context.Context) error {
		var err error
		created, err = s.repo.Create(ctx, c)
		return err
	}
	if !p.IsAdmin {
		if err := listing.Owner(ctx, s.listings, s.logger, listingID, p.SellerID); err != nil {
			return store.AdCampaign{}, err
		}
		if err := insert(ctx); err != nil {
			return store.AdCampaign{}, err
		}
		return created, nil
	}
	c.ID = store.NewID()
	ev := auditx.Event{
		ActorID:    p.SellerID,
		Action:     "ad_campaign.admin_create",
		TargetType: "ad_campaign",
		TargetID:   c.ID,
		Metadata: map[string]string{
			"seller_id":  sellerID,
			"listing_id": listingID,
			"budget":     strconv.FormatInt(p.Budget, 10),
		},
	}
	if err := s.audit.Do(ctx, ev, insert); err != nil {
		return store.AdCampaign{}, err
	}
	return created, nil
}

// ListSponsoredSlots returns the listing ids to render in sponsored slots for a
// placement context, ordered best-bid first. The context key is accepted for
// forward compatibility (real placement targeting is out of scope for the mock);
// today it returns every active campaign's listing, deduplicated, keeping the
// highest-bid entry per listing. An empty/anonymous context never panics.
func (s *Service) ListSponsoredSlots(ctx context.Context, _ string) ([]string, error) {
	campaigns, err := s.repo.ListActive(ctx, defaultPageSize)
	if err != nil {
		return nil, err
	}
	// campaigns arrive bid-descending from the store. Dedup by listing_id,
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
