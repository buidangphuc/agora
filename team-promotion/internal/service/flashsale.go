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
)

// FlashSaleService holds the flash-sale campaign business logic.
type FlashSaleService struct {
	campaigns repository.FlashSaleRepository
	emitter   *producer.Emitter
	flags     featureflags.Evaluator
	logger    *slog.Logger
	nowFn     func() time.Time
}

// NewFlashSaleService wires the campaign repository, the promotion.events producer
// (may be nil when Kafka is disabled) and the feature flag evaluator.
func NewFlashSaleService(
	campaigns repository.FlashSaleRepository,
	prod *bootstrap.EventProducer,
	flags featureflags.Evaluator,
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
		logger:    logger,
		nowFn:     time.Now,
	}
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
}

// CreateCampaign persists a campaign and emits FlashSaleChanged on promotion.events.
func (s *FlashSaleService) CreateCampaign(ctx context.Context, p CreateCampaignParams) (repository.FlashSaleCampaign, error) {
	if strings.TrimSpace(p.ListingID) == "" {
		return repository.FlashSaleCampaign{}, ErrInvalidCampaign
	}
	c := repository.FlashSaleCampaign{
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

// GetFlashSaleStock returns remaining = stock_cap - stock_sold (floored at 0) plus
// the cap, for the live banner/meter.
func (s *FlashSaleService) GetFlashSaleStock(ctx context.Context, campaignID string) (remaining, stockCap int64, err error) {
	c, err := s.campaigns.GetByID(ctx, campaignID)
	if err != nil {
		return 0, 0, err
	}
	remaining = c.StockCap - c.StockSold
	if remaining < 0 {
		remaining = 0
	}
	return remaining, c.StockCap, nil
}

// flashSaleEnabled evaluates the kill-switch, failing open (true) on any provider
// error so a Flipt outage never disables promotions.
func (s *FlashSaleService) flashSaleEnabled(ctx context.Context) bool {
	if s.flags == nil {
		return true
	}
	return s.flags.BooleanEnabled(ctx, featureflags.FlagFlashSaleEnabled, true)
}
