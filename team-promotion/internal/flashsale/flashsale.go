package flashsale

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/flags"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/team-promotion/internal/listing"
	"github.com/buidangphuc/team-promotion/internal/store"
)

const FlagFlashSaleEnabled = "flash-sale-enabled"

const defaultPageSize = 50

type Service struct {
	campaigns store.FlashSaleRepository
	holds     store.FlashHoldRepository
	flags     flags.Evaluator
	listings  listing.Getter
	audit     *auditx.Gate
	event     func(context.Context, store.FlashSaleCampaign) (outbox.Row, error)
	logger    *slog.Logger
	nowFn     func() time.Time
}

func New(
	campaigns store.FlashSaleRepository,
	holds store.FlashHoldRepository,
	flags flags.Evaluator,
	listings listing.Getter,
	audit *auditx.Gate,
	event func(context.Context, store.FlashSaleCampaign) (outbox.Row, error),
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		campaigns: campaigns,
		holds:     holds,
		flags:     flags,
		listings:  listings,
		audit:     audit,
		event:     event,
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

	// CallerID is the authenticated principal id; IsAdmin skips the ownership check.
	CallerID string
	IsAdmin  bool

	IdempotencyKey string
}

type idempotentCreator interface {
	LookupIdempotent(ctx context.Context, actorID, key string, now time.Time) (store.IdempotentCampaign, error)
	CreateIdempotent(ctx context.Context, actorID, key, payloadHash string, c store.FlashSaleCampaign, now time.Time, event func(store.FlashSaleCampaign) (outbox.Row, error)) (store.FlashSaleCampaign, bool, error)
}

func invalidField(field, why string) error {
	return fmt.Errorf("%w: %s %s", ErrInvalidCampaign, field, why)
}

func payloadHash(c store.FlashSaleCampaign) string {
	raw := strings.Join([]string{
		c.ListingID, c.VariantID,
		strconv.FormatInt(c.SalePrice, 10), strconv.FormatInt(c.StockCap, 10),
		strconv.FormatInt(c.StartsAt.UnixNano(), 10), strconv.FormatInt(c.EndsAt.UnixNano(), 10),
	}, "\x1f")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *Service) validateFields(c store.FlashSaleCampaign) error {
	if c.SalePrice <= 0 {
		return invalidField("sale_price", "must be positive")
	}
	if c.StockCap <= 0 {
		return invalidField("stock_cap", "must be positive")
	}
	if !c.EndsAt.After(c.StartsAt) {
		return invalidField("ends_at", "must be after starts_at")
	}
	if !c.EndsAt.After(s.nowFn()) {
		return invalidField("ends_at", "must be in the future")
	}
	return nil
}

func (s *Service) CreateCampaign(ctx context.Context, p CreateCampaignParams) (store.FlashSaleCampaign, error) {
	listingID := strings.TrimSpace(p.ListingID)
	if listingID == "" {
		return store.FlashSaleCampaign{}, invalidField("listing_id", "is required")
	}
	c := store.FlashSaleCampaign{
		ListingID: listingID,
		VariantID: p.VariantID,
		SalePrice: p.SalePrice,
		StockCap:  p.StockCap,
		StartsAt:  p.StartsAt,
		EndsAt:    p.EndsAt,
	}
	idem, useKey := s.campaigns.(idempotentCreator)
	useKey = useKey && p.IsAdmin && p.IdempotencyKey != ""
	hash := payloadHash(c)
	now := s.nowFn()
	if useKey {
		hit, err := idem.LookupIdempotent(ctx, p.CallerID, p.IdempotencyKey, now)
		switch {
		case err == nil:
			if hit.PayloadHash != hash {
				return store.FlashSaleCampaign{}, store.ErrIdempotencyMismatch
			}
			return hit.Campaign, nil
		case !errors.Is(err, store.ErrIdempotencyNotFound):
			return store.FlashSaleCampaign{}, err
		}
	}
	if err := s.validateFields(c); err != nil {
		return store.FlashSaleCampaign{}, err
	}
	if !p.IsAdmin || useKey {
		l, err := listing.Get(ctx, s.listings, s.logger, listingID)
		if err != nil {
			if errors.Is(err, listing.ErrNotFound) {
				return store.FlashSaleCampaign{}, invalidField("listing_id", "not found")
			}
			return store.FlashSaleCampaign{}, err
		}
		if !p.IsAdmin {
			if err := listing.CheckOwner(l, p.CallerID); err != nil {
				return store.FlashSaleCampaign{}, err
			}
		}
		if c.SalePrice >= l.GetPrice() {
			return store.FlashSaleCampaign{}, invalidField("sale_price", "must be below the listing price")
		}
	}
	var created store.FlashSaleCampaign
	eventFn := func(ctx context.Context) func(store.FlashSaleCampaign) (outbox.Row, error) {
		return func(cc store.FlashSaleCampaign) (outbox.Row, error) {
			return s.event(ctx, cc)
		}
	}
	if !p.IsAdmin {
		var err error
		created, err = s.campaigns.Create(ctx, c, eventFn(ctx))
		if err != nil {
			return store.FlashSaleCampaign{}, err
		}
		return created, nil
	}
	c.ID = store.NewID()
	ev := auditx.Event{
		ActorID:    p.CallerID,
		Action:     "flash_sale.admin_create",
		TargetType: "flash_sale",
		TargetID:   c.ID,
		Metadata:   map[string]string{"listing_id": listingID},
	}
	insert := func(ctx context.Context) error {
		var err error
		if useKey {
			created, _, err = idem.CreateIdempotent(ctx, p.CallerID, p.IdempotencyKey, hash, c, now, eventFn(ctx))
		} else {
			created, err = s.campaigns.Create(ctx, c, eventFn(ctx))
		}
		return err
	}
	if err := s.audit.Do(ctx, ev, insert); err != nil {
		return store.FlashSaleCampaign{}, err
	}
	return created, nil
}

// GetActiveFlashSale returns the active campaign for a listing, honoring the sale
// window. The flash-sale kill-switch (FlagFlashSaleEnabled, fail-open) suppresses
// the sale when a human turns it OFF during an incident.
func (s *Service) GetActiveFlashSale(ctx context.Context, listingID string) (store.FlashSaleCampaign, bool, error) {
	if !s.flashSaleEnabled(ctx) {
		return store.FlashSaleCampaign{}, false, nil
	}
	c, err := s.campaigns.GetActiveByListing(ctx, listingID, s.nowFn())
	if err != nil {
		if errors.Is(err, store.ErrCampaignNotFound) {
			return store.FlashSaleCampaign{}, false, nil
		}
		return store.FlashSaleCampaign{}, false, err
	}
	return c, true, nil
}

// ListActiveCampaigns returns a page of currently-active campaigns. The cursor is
// a stringified offset; nextCursor is empty on the last page.
func (s *Service) ListActiveCampaigns(ctx context.Context, cursor string, pageSize int32) ([]store.FlashSaleCampaign, string, error) {
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
func (s *Service) GetFlashSaleStock(ctx context.Context, campaignID string) (remaining, stockCap int64, err error) {
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

var errCancelNotConfigured = errors.New("flash sale cancel is not supported by the repository")

// replayOutcome maps a stored hold to the Reserve outcome: a live or committed hold
// is reserved, a released or expired one is not active.
func (s *Service) replayOutcome(h store.FlashHold) (bool, string) {
	switch h.Status {
	case store.FlashHoldCommitted:
		return true, ""
	case store.FlashHoldReserved:
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
func (s *Service) ReserveFlashSaleStock(ctx context.Context, reservationID, campaignID, listingID, variantID string, quantity, expectedSalePrice int64) (bool, string, error) {
	if s.holds == nil {
		return false, "", errHoldsNotConfigured
	}
	stored, err := s.holds.Get(ctx, reservationID)
	if err == nil {
		reserved, reason := s.replayOutcome(stored)
		return reserved, reason, nil
	}
	if !errors.Is(err, store.ErrFlashHoldNotFound) {
		return false, "", err
	}
	if !s.flashSaleEnabled(ctx) {
		return false, reasonNotActive, nil
	}
	c, err := s.campaigns.GetByID(ctx, campaignID)
	if err != nil {
		if errors.Is(err, store.ErrCampaignNotFound) {
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
	stored, created, err := s.holds.Reserve(ctx, store.FlashHold{
		ReservationID: reservationID, CampaignID: campaignID, ListingID: listingID, VariantID: variantID, Quantity: quantity,
	}, now)
	switch {
	case errors.Is(err, store.ErrFlashStockInsufficient):
		return false, reasonInsufficientStock, nil
	case errors.Is(err, store.ErrCampaignNotFound), errors.Is(err, store.ErrCampaignCancelled):
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
func (s *Service) CommitFlashSaleStock(ctx context.Context, reservationID string) (bool, error) {
	if s.holds == nil {
		return false, errHoldsNotConfigured
	}
	return s.holds.Commit(ctx, reservationID, s.nowFn())
}

// ReleaseFlashSaleStock returns a hold's units (also after commit). Idempotent;
// false for an unknown reservation.
func (s *Service) ReleaseFlashSaleStock(ctx context.Context, reservationID string) (bool, error) {
	if s.holds == nil {
		return false, errHoldsNotConfigured
	}
	return s.holds.Release(ctx, reservationID, s.nowFn())
}

// flashSaleEnabled evaluates the kill-switch, failing open (true) on any provider
// error so a Flipt outage never disables promotions.
func (s *Service) flashSaleEnabled(ctx context.Context) bool {
	if s.flags == nil {
		return true
	}
	return s.flags.BooleanEnabled(ctx, FlagFlashSaleEnabled, true)
}
