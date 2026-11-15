package sponsored

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"google.golang.org/genproto/googleapis/rpc/code"
	rpcstatus "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/status"

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
	minBid    int64
	maxBudget int64
	now       func() time.Time
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
	return &Service{repo: repo, listings: listings, audit: audit, logger: logger, maxBid: DefaultMaxAdBid, maxBudget: DefaultMaxAdBudget, minBid: DefaultMinAdBid, now: time.Now}
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
	if p.Budget < 0 || p.Bid < 0 || p.Budget > s.maxBudget || p.Bid > s.maxBid || p.Bid < s.minBid || p.Budget < p.Bid {
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
		l, err := listing.Get(ctx, s.listings, s.logger, listingID)
		if err != nil {
			return store.AdCampaign{}, err
		}
		if err := listing.CheckOwner(l, p.SellerID); err != nil {
			return store.AdCampaign{}, err
		}
		c.CategoryID = l.GetCategoryId()
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

const (
	DefaultMinAdBid  int64 = 500
	defaultSlotLimit       = 2
	maxSlotLimit           = 4
	servableFetch          = 200
)

type SlotsParams struct {
	ContextStr string
	Placement  string
	CategoryID string
	Limit      int32
}

type Slot struct {
	ListingID  string
	CampaignID string
	Token      string
}

type SlotsResult struct {
	ListingIDs []string
	Slots      []Slot
}

func (s *Service) WithMinBid(minBid int64) *Service {
	if minBid > 0 {
		s.minBid = minBid
	}
	return s
}

func (s *Service) ListSponsoredSlots(ctx context.Context, p SlotsParams) (SlotsResult, error) {
	switch p.Placement {
	case "":
		return s.legacySlots(ctx)
	case "search", "home":
	default:
		return SlotsResult{}, ErrInvalidAdCampaign
	}
	limit := int(p.Limit)
	if limit < 0 || limit > maxSlotLimit {
		return SlotsResult{}, ErrInvalidAdCampaign
	}
	if limit == 0 {
		limit = defaultSlotLimit
	}
	category := ""
	if p.Placement == "search" {
		category = strings.TrimSpace(p.CategoryID)
		if category == "" {
			return SlotsResult{}, nil
		}
	}
	rows, err := s.repo.ListServable(ctx, category, servableFetch)
	if err != nil {
		return SlotsResult{}, errSlotsUnavailable()
	}
	seen := map[string]bool{}
	var slots []Slot
	var issues []store.ImpressionIssue
	for _, c := range rows {
		if len(slots) == limit {
			break
		}
		if seen[c.ListingID] {
			continue
		}
		seen[c.ListingID] = true
		token, err := newToken()
		if err != nil {
			return SlotsResult{}, errSlotsUnavailable()
		}
		slots = append(slots, Slot{ListingID: c.ListingID, CampaignID: c.ID, Token: token})
		issues = append(issues, store.ImpressionIssue{Token: token, CampaignID: c.ID, ListingID: c.ListingID})
	}
	if len(slots) == 0 {
		return SlotsResult{}, nil
	}
	if err := s.repo.IssueImpressions(ctx, issues); err != nil {
		s.logger.WarnContext(ctx, "issue ad impressions failed", "err", err)
		return SlotsResult{}, errSlotsUnavailable()
	}
	return SlotsResult{Slots: slots}, nil
}

func (s *Service) legacySlots(ctx context.Context) (SlotsResult, error) {
	rows, err := s.repo.ListActive(ctx, servableFetch)
	if err != nil {
		return SlotsResult{}, err
	}
	seen := map[string]bool{}
	var ids []string
	for _, c := range rows {
		if seen[c.ListingID] {
			continue
		}
		seen[c.ListingID] = true
		ids = append(ids, c.ListingID)
		if len(ids) == defaultPageSize {
			break
		}
	}
	return SlotsResult{ListingIDs: ids}, nil
}

func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func errSlotsUnavailable() error {
	return status.ErrorProto(&rpcstatus.Status{Code: int32(code.Code_UNAVAILABLE), Message: "sponsored slots unavailable"})
}

type RecordClickParams struct {
	Token       string
	PrincipalID string
}

type ClickResult struct {
	ListingID string
	Charged   bool
}

func (s *Service) WithClock(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

func (s *Service) RecordAdClick(ctx context.Context, p RecordClickParams) (ClickResult, error) {
	if strings.TrimSpace(p.Token) == "" {
		return ClickResult{}, ErrInvalidAdCampaign
	}
	res, err := s.repo.RecordClick(ctx, p.Token, p.PrincipalID, s.now())
	if err != nil {
		if errors.Is(err, store.ErrImpressionNotFound) {
			return ClickResult{}, err
		}
		s.logger.WarnContext(ctx, "record ad click failed", "err", err)
		return ClickResult{}, errSlotsUnavailable()
	}
	return ClickResult{ListingID: res.ListingID, Charged: res.Charged}, nil
}

// RED-PHASE STUBS (T2-service-rpc): the implementer replaces these.

type ListMyAdCampaignsParams struct {
	SellerID string
	PageSize int32
	Cursor   string
}

type ListMyAdCampaignsResult struct {
	Campaigns  []store.AdCampaign
	NextCursor string
}

var errCampaignMgmtNotImplemented = errors.New("campaign management not implemented")

func (s *Service) ListMyAdCampaigns(_ context.Context, _ ListMyAdCampaignsParams) (ListMyAdCampaignsResult, error) {
	return ListMyAdCampaignsResult{}, errCampaignMgmtNotImplemented
}

func (s *Service) PauseAdCampaign(_ context.Context, _, _ string) (store.AdCampaign, error) {
	return store.AdCampaign{}, errCampaignMgmtNotImplemented
}

func (s *Service) ResumeAdCampaign(_ context.Context, _, _ string) (store.AdCampaign, error) {
	return store.AdCampaign{}, errCampaignMgmtNotImplemented
}
