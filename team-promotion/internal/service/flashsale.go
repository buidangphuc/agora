package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/buidangphuc/team-promotion/internal/bootstrap"
	"github.com/buidangphuc/team-promotion/internal/featureflags"
	"github.com/buidangphuc/team-promotion/internal/producer"
	"github.com/buidangphuc/team-promotion/internal/repository"
	"github.com/buidangphuc/team-promotion/internal/upstream"
)

// FlashSaleService holds the flash-sale campaign business logic.
type FlashSaleService struct {
	campaigns repository.FlashSaleRepository
	emitter   *producer.Emitter
	flags     featureflags.Evaluator
	listings  upstream.ListingGetter // nil when UPSTREAM_DOMAIN_ADDR is unset
	logger    *slog.Logger
	nowFn     func() time.Time
	audit     AuditWriter // nil = admin campaign creation fails closed
	holds     repository.FlashHoldRepository
}

// NewFlashSaleService wires the campaign repository, the promotion.events producer
// (may be nil when Kafka is disabled), the feature flag evaluator and the team-domain
// listing client used to verify campaign ownership (nil = fail closed for sellers).
func NewFlashSaleService(
	campaigns repository.FlashSaleRepository,
	prod *bootstrap.EventProducer,
	flags featureflags.Evaluator,
	listings upstream.ListingGetter,
	logger *slog.Logger,
) *FlashSaleService {
	if logger == nil {
		logger = slog.Default()
	}
	var pub producer.Publisher
	if prod != nil {
		pub = prod
	}
	return &FlashSaleService{
		campaigns: campaigns,
		emitter:   producer.NewEmitter(pub, logger),
		flags:     flags,
		listings:  listings,
		logger:    logger,
		nowFn:     time.Now,
	}
}

// WithAudit sets the audit writer used for admin campaign creation.
func (s *FlashSaleService) WithAudit(w AuditWriter) *FlashSaleService {
	s.audit = w
	return s
}

// WithHolds sets the flash-stock hold repository used by the reserve/commit/release
// RPCs and by GetFlashSaleStock.
func (s *FlashSaleService) WithHolds(h repository.FlashHoldRepository) *FlashSaleService {
	s.holds = h
	return s
}

// ErrInvalidCampaign is returned for malformed CreateCampaign input.
var ErrInvalidCampaign = errors.New("invalid flash sale campaign")

// CreateCampaignParams is the validated input for CreateCampaign.
type CreateCampaignParams struct {
	ListingID string
	VariantID string
	SalePrice int64
	StockCap  int64
	StartsAt  time.Time
	EndsAt    time.Time

	// CallerID is the authenticated principal id; IsAdmin skips the ownership check.
	CallerID string
	IsAdmin  bool
}

// CreateCampaign persists a campaign and emits FlashSaleChanged on promotion.events.
// Non-admin callers must own the listing (checked against team-domain). Errors from
// the ownership check are gRPC status errors (InvalidArgument, PermissionDenied,
// Unavailable).
func (s *FlashSaleService) CreateCampaign(ctx context.Context, p CreateCampaignParams) (repository.FlashSaleCampaign, error) {
	if strings.TrimSpace(p.ListingID) == "" {
		return repository.FlashSaleCampaign{}, ErrInvalidCampaign
	}
	var id string
	if p.IsAdmin {
		id = newCampaignID()
		if err := writeAudit(ctx, s.audit, upstream.AuditEvent{
			ActorID:    p.CallerID,
			Action:     "flash_sale.admin_create",
			TargetType: "flash_sale",
			TargetID:   id,
			Metadata:   map[string]string{"listing_id": strings.TrimSpace(p.ListingID)},
		}); err != nil {
			return repository.FlashSaleCampaign{}, err
		}
	} else if err := s.requireListingOwner(ctx, strings.TrimSpace(p.ListingID), p.CallerID); err != nil {
		return repository.FlashSaleCampaign{}, err
	}
	c := repository.FlashSaleCampaign{
		ID:        id,
		ListingID: strings.TrimSpace(p.ListingID),
		VariantID: p.VariantID,
		SalePrice: p.SalePrice,
		StockCap:  p.StockCap,
		StartsAt:  p.StartsAt,
		EndsAt:    p.EndsAt,
	}
	created, err := s.campaigns.Create(ctx, c)
	if err != nil {
		return repository.FlashSaleCampaign{}, err
	}
	if err := s.emitter.EmitFlashSaleChanged(ctx, CampaignToProto(created)); err != nil {
		s.logger.Warn("emit FlashSaleChanged failed", slog.String("campaign_id", created.ID), slog.Any("err", err))
	}
	return created, nil
}

func (s *FlashSaleService) requireListingOwner(ctx context.Context, listingID, callerID string) error {
	return requireListingOwner(ctx, s.listings, s.logger, listingID, callerID)
}

// GetActiveFlashSale returns the active campaign for a listing, honoring the sale
// window. The flash-sale kill-switch (FlagFlashSaleEnabled, fail-open) suppresses
// the sale when a human turns it OFF during an incident.
func (s *FlashSaleService) GetActiveFlashSale(ctx context.Context, listingID string) (repository.FlashSaleCampaign, bool, error) {
	if !s.flashSaleEnabled(ctx) {
		return repository.FlashSaleCampaign{}, false, nil
	}
	c, err := s.campaigns.GetActiveByListing(ctx, listingID, s.nowFn())
	if err != nil {
		if errors.Is(err, repository.ErrCampaignNotFound) {
			return repository.FlashSaleCampaign{}, false, nil
		}
		return repository.FlashSaleCampaign{}, false, err
	}
	return c, true, nil
}

// ListActiveCampaigns returns a page of currently-active campaigns. The cursor is
// a stringified offset; nextCursor is empty on the last page.
func (s *FlashSaleService) ListActiveCampaigns(ctx context.Context, cursor string, pageSize int32) ([]repository.FlashSaleCampaign, string, error) {
	if !s.flashSaleEnabled(ctx) {
		return nil, "", nil
	}
	limit := int(pageSize)
	if limit <= 0 || limit > 200 {
		limit = defaultPageSize
	}
	offset := 0
	if cursor != "" {
		if n, err := strconv.Atoi(cursor); err == nil && n >= 0 {
			offset = n
		}
	}
	items, err := s.campaigns.ListActive(ctx, s.nowFn(), limit+1, offset)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = strconv.Itoa(offset + limit)
	}
	return items, next, nil
}

// GetFlashSaleStock returns remaining = stock_cap - stock_sold - active holds (floored
// at 0) plus the cap, for the live banner/meter.
func (s *FlashSaleService) GetFlashSaleStock(ctx context.Context, campaignID string) (remaining, stockCap int64, err error) {
	c, err := s.campaigns.GetByID(ctx, campaignID)
	if err != nil {
		return 0, 0, err
	}
	var held int64
	if s.holds != nil {
		if held, err = s.holds.ActiveHeld(ctx, campaignID, s.nowFn()); err != nil {
			return 0, 0, err
		}
	}
	remaining = c.StockCap - c.StockSold - held
	if remaining < 0 {
		remaining = 0
	}
	return remaining, c.StockCap, nil
}

// Closed set of Reserve refusal reasons.
const (
	reasonNotActive         = "not_active"
	reasonPriceChanged      = "price_changed"
	reasonInsufficientStock = "insufficient_stock"
)

var errHoldsNotConfigured = errors.New("flash sale holds are not configured")

// replayOutcome maps a stored hold to the Reserve outcome: a live or committed hold
// is reserved, a released or expired one is not active.
func (s *FlashSaleService) replayOutcome(h repository.FlashHold) (bool, string) {
	switch h.Status {
	case repository.FlashHoldCommitted:
		return true, ""
	case repository.FlashHoldReserved:
		if s.nowFn().Before(h.ExpiresAt) {
			return true, ""
		}
	}
	return false, reasonNotActive
}

// ReserveFlashSaleStock holds quantity units of a campaign under reservationID.
// Idempotent on reservationID: a replay returns the stored outcome whatever the other
// fields say. The first failing check decides the reason: not_active, price_changed,
// insufficient_stock.
func (s *FlashSaleService) ReserveFlashSaleStock(ctx context.Context, reservationID, campaignID, listingID, variantID string, quantity, expectedSalePrice int64) (bool, string, error) {
	if s.holds == nil {
		return false, "", errHoldsNotConfigured
	}
	stored, err := s.holds.Get(ctx, reservationID)
	if err == nil {
		reserved, reason := s.replayOutcome(stored)
		return reserved, reason, nil
	}
	if !errors.Is(err, repository.ErrFlashHoldNotFound) {
		return false, "", err
	}
	if !s.flashSaleEnabled(ctx) {
		return false, reasonNotActive, nil
	}
	c, err := s.campaigns.GetByID(ctx, campaignID)
	if err != nil {
		if errors.Is(err, repository.ErrCampaignNotFound) {
			return false, reasonNotActive, nil
		}
		return false, "", err
	}
	now := s.nowFn()
	if (!c.StartsAt.IsZero() && now.Before(c.StartsAt)) || (!c.EndsAt.IsZero() && !now.Before(c.EndsAt)) ||
		c.ListingID != listingID || (c.VariantID != "" && c.VariantID != variantID) {
		return false, reasonNotActive, nil
	}
	if c.SalePrice != expectedSalePrice {
		return false, reasonPriceChanged, nil
	}
	stored, created, err := s.holds.Reserve(ctx, repository.FlashHold{
		ReservationID: reservationID, CampaignID: campaignID, ListingID: listingID, VariantID: variantID, Quantity: quantity,
	}, now)
	switch {
	case errors.Is(err, repository.ErrFlashStockInsufficient):
		return false, reasonInsufficientStock, nil
	case errors.Is(err, repository.ErrCampaignNotFound):
		return false, reasonNotActive, nil
	case err != nil:
		return false, "", err
	case !created:
		reserved, reason := s.replayOutcome(stored)
		return reserved, reason, nil
	}
	return true, "", nil
}

// CommitFlashSaleStock turns a live hold into sold units. Idempotent; false for an
// unknown, released or expired hold. The sale window is not checked.
func (s *FlashSaleService) CommitFlashSaleStock(ctx context.Context, reservationID string) (bool, error) {
	if s.holds == nil {
		return false, errHoldsNotConfigured
	}
	return s.holds.Commit(ctx, reservationID, s.nowFn())
}

// ReleaseFlashSaleStock returns a hold's units (also after commit). Idempotent;
// false for an unknown reservation.
func (s *FlashSaleService) ReleaseFlashSaleStock(ctx context.Context, reservationID string) (bool, error) {
	if s.holds == nil {
		return false, errHoldsNotConfigured
	}
	return s.holds.Release(ctx, reservationID, s.nowFn())
}

// flashSaleEnabled evaluates the kill-switch, failing open (true) on any provider
// error so a Flipt outage never disables promotions.
func (s *FlashSaleService) flashSaleEnabled(ctx context.Context) bool {
	if s.flags == nil {
		return true
	}
	return s.flags.BooleanEnabled(ctx, featureflags.FlagFlashSaleEnabled, true)
}
