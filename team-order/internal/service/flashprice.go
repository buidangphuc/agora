package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"

	"google.golang.org/grpc"

	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/repository"
)

var (
	// ErrPriceChanged: a cart line's price no longer matches the advertised price
	// (flash sale started, ended, or its hold was refused). The lines were
	// re-priced; nothing is held and no order exists. -> FAILED_PRECONDITION.
	ErrPriceChanged = errors.New("price changed")
	// ErrPromotionUnavailable: a flash-priced line could not be confirmed or held
	// because team-promotion failed. Nothing is placed. -> UNAVAILABLE.
	ErrPromotionUnavailable = errors.New("promotion service unavailable")
)

// OrderFlashClient is the part of team-promotion's FlashSaleService checkout
// needs: the price lookup and the three stock-hold RPCs.
type OrderFlashClient interface {
	GetActiveFlashSale(ctx context.Context, in *promotionv1.GetActiveFlashSaleRequest, opts ...grpc.CallOption) (*promotionv1.GetActiveFlashSaleResponse, error)
	ReserveFlashSaleStock(ctx context.Context, in *promotionv1.ReserveFlashSaleStockRequest, opts ...grpc.CallOption) (*promotionv1.ReserveFlashSaleStockResponse, error)
	CommitFlashSaleStock(ctx context.Context, in *promotionv1.CommitFlashSaleStockRequest, opts ...grpc.CallOption) (*promotionv1.CommitFlashSaleStockResponse, error)
	ReleaseFlashSaleStock(ctx context.Context, in *promotionv1.ReleaseFlashSaleStockRequest, opts ...grpc.CallOption) (*promotionv1.ReleaseFlashSaleStockResponse, error)
}

// WithOrderFlashSaleClient prices and holds flash-sale lines at checkout. A nil or
// typed-nil client means "none": every line is priced at the regular price and no
// hold, commit or release call is made.
func WithOrderFlashSaleClient(c OrderFlashClient) OrderServiceOption {
	return func(s *OrderService) {
		if c == nil {
			return
		}
		if v := reflect.ValueOf(c); v.Kind() == reflect.Ptr && v.IsNil() {
			return
		}
		s.orderFlash = c
	}
}

// FlashReservationID is the team-promotion hold id of one order line.
func FlashReservationID(orderID, listingID, variantID string) string {
	return orderID + ":" + listingID + ":" + variantID
}

// flashLine is a cart line charged at an active flash sale price.
type flashLine struct {
	campaignID string
	regular    int64
}

// flashHold is a hold recorded in Phase A (or sent and unconfirmed).
type flashHold struct {
	reservationID string
}

// regularPrice is the line's non-flash price: the variant price when > 0, else the
// listing price.
func regularPrice(l *listingv1.Listing, variantID string) int64 {
	p := l.GetPrice()
	if variantID != "" {
		for _, v := range l.GetVariants() {
			if v.GetId() == variantID && v.GetPrice() > 0 {
				return v.GetPrice()
			}
		}
	}
	return p
}

type flashLookup struct {
	resp *promotionv1.GetActiveFlashSaleResponse
	err  error
}

// advertisedPrice mirrors CartService.flashPrice: the active campaign's sale price
// when valid (0 < sale < regular, campaign variant empty or equal), else regular.
func advertisedPrice(resp *promotionv1.GetActiveFlashSaleResponse, variantID string, regular int64) (price int64, campaignID string) {
	c := resp.GetCampaign()
	if !resp.GetActive() || c == nil {
		return regular, ""
	}
	if c.GetVariantId() != "" && c.GetVariantId() != variantID {
		return regular, ""
	}
	if p := c.GetSalePrice(); p > 0 && p < regular {
		return p, c.GetId()
	}
	return regular, ""
}

// precheckPrices runs before the saga exists. It re-derives each selected line's
// advertised price and compares it with the cart price. Any differing line is
// re-priced in the buyer's cart and ErrPriceChanged is returned (nothing held).
// On success it returns the flash lines keyed by cart item id.
//
// A regular price <= 0 means the listing price is unknown: there is nothing to
// compare against, so the line proceeds at its cart price with no flash handling.
func (s *OrderService) precheckPrices(ctx context.Context, buyerID string, items []repository.CartItem) (map[string]flashLine, error) {
	listings := map[string]*listingv1.Listing{}
	lookups := map[string]flashLookup{}
	for _, it := range items {
		if _, ok := listings[it.ListingID]; ok {
			continue
		}
		resp, err := s.domainClient.GetListing(ctx, &listingv1.GetListingRequest{Id: it.ListingID})
		if err != nil {
			return nil, fmt.Errorf("get listing %q from domain: %w", it.ListingID, err)
		}
		listings[it.ListingID] = resp.GetListing()
		if s.orderFlash != nil && regularPriceKnown(resp.GetListing(), items, it.ListingID) {
			r, ferr := s.orderFlash.GetActiveFlashSale(ctx, &promotionv1.GetActiveFlashSaleRequest{ListingId: it.ListingID})
			lookups[it.ListingID] = flashLookup{resp: r, err: ferr}
		}
	}

	flash := map[string]flashLine{}
	fixes := map[string]int64{} // cart item id -> corrected price
	unavailable := false
	for _, it := range items {
		regular := regularPrice(listings[it.ListingID], it.VariantID)
		if regular <= 0 {
			continue
		}
		want, campaign := regular, ""
		if lk, ok := lookups[it.ListingID]; ok {
			if lk.err != nil {
				s.logger.WarnContext(ctx, "flash sale lookup failed at checkout",
					slog.String("listing_id", it.ListingID), slog.Any("err", lk.err))
				switch {
				case it.UnitPrice > regular:
					fixes[it.ID] = regular
				case it.UnitPrice < regular:
					unavailable = true
				}
				continue
			}
			want, campaign = advertisedPrice(lk.resp, it.VariantID, regular)
		}
		if it.UnitPrice != want {
			fixes[it.ID] = want
			continue
		}
		if campaign != "" {
			flash[it.ID] = flashLine{campaignID: campaign, regular: regular}
		}
	}

	if len(fixes) > 0 {
		for id, price := range fixes {
			if err := s.cartRepo.UpdateItemPrice(ctx, buyerID, id, price); err != nil {
				s.logger.ErrorContext(ctx, "failed to re-price cart line",
					slog.String("item_id", id), slog.Any("err", err))
			}
		}
		return nil, ErrPriceChanged
	}
	if unavailable {
		return nil, ErrPromotionUnavailable
	}
	return flash, nil
}

// regularPriceKnown reports whether any selected line of the listing has a known
// regular price (otherwise no flash lookup is needed).
func regularPriceKnown(l *listingv1.Listing, items []repository.CartItem, listingID string) bool {
	for _, it := range items {
		if it.ListingID == listingID && regularPrice(l, it.VariantID) > 0 {
			return true
		}
	}
	return false
}

// holdFlashLine reserves the flash stock of one line (Phase A). A refusal re-prices
// the line to its regular price (ErrPriceChanged); a transport error is
// ErrPromotionUnavailable. The hold is recorded whenever it may exist.
func (s *OrderService) holdFlashLine(ctx context.Context, buyerID string, g *sellerGroup, it repository.CartItem, fl flashLine) error {
	id := FlashReservationID(g.orderID, it.ListingID, it.VariantID)
	resp, err := s.orderFlash.ReserveFlashSaleStock(ctx, &promotionv1.ReserveFlashSaleStockRequest{
		ReservationId:     id,
		CampaignId:        fl.campaignID,
		ListingId:         it.ListingID,
		VariantId:         it.VariantID,
		Quantity:          int64(it.Quantity),
		ExpectedSalePrice: it.UnitPrice,
	})
	if err != nil {
		// The hold may have been applied; release it by id on compensation.
		g.flashHolds = append(g.flashHolds, flashHold{reservationID: id})
		s.logger.WarnContext(ctx, "flash stock hold failed", slog.String("listing_id", it.ListingID), slog.Any("err", err))
		return fmt.Errorf("%w: %v", ErrPromotionUnavailable, err)
	}
	if !resp.GetReserved() {
		s.logger.WarnContext(ctx, "flash stock hold refused",
			slog.String("listing_id", it.ListingID), slog.String("reason", resp.GetReason()))
		if uerr := s.cartRepo.UpdateItemPrice(ctx, buyerID, it.ID, fl.regular); uerr != nil {
			s.logger.ErrorContext(ctx, "failed to re-price cart line",
				slog.String("item_id", it.ID), slog.Any("err", uerr))
		}
		return ErrPriceChanged
	}
	g.flashHolds = append(g.flashHolds, flashHold{reservationID: id})
	return nil
}

// commitFlashHolds confirms every hold (Phase B, before the domain commit).
func (s *OrderService) commitFlashHolds(ctx context.Context, groups []*sellerGroup) error {
	for _, g := range groups {
		for _, h := range g.flashHolds {
			resp, err := s.orderFlash.CommitFlashSaleStock(ctx, &promotionv1.CommitFlashSaleStockRequest{ReservationId: h.reservationID})
			if err != nil {
				return fmt.Errorf("%w: commit flash hold %s: %v", ErrPromotionUnavailable, h.reservationID, err)
			}
			if !resp.GetCommitted() {
				return fmt.Errorf("%w: flash hold %s not committed", ErrPromotionUnavailable, h.reservationID)
			}
		}
	}
	return nil
}

// releaseFlashHolds releases every recorded hold, best effort, on a fresh context.
func (s *OrderService) releaseFlashHolds(groups []*sellerGroup) {
	if s.orderFlash == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.releaseCfg.timeout)
	defer cancel()
	for _, g := range groups {
		for _, h := range g.flashHolds {
			if _, err := s.orderFlash.ReleaseFlashSaleStock(ctx, &promotionv1.ReleaseFlashSaleStockRequest{ReservationId: h.reservationID}); err != nil {
				s.logger.ErrorContext(ctx, "flash hold release failed",
					slog.String("reservation_id", h.reservationID), slog.Any("err", err))
			}
		}
	}
}

// releaseOrderFlashHolds releases the flash hold of every item of a cancelled
// order (best effort: a failure is logged and never fails the cancel; releasing a
// hold that never existed is a no-op in team-promotion).
func (s *OrderService) releaseOrderFlashHolds(ctx context.Context, order repository.Order) {
	if s.orderFlash == nil {
		return
	}
	items := order.Items
	if len(items) == 0 {
		full, err := s.orderRepo.GetOrder(ctx, order.ID)
		if err != nil {
			s.logger.ErrorContext(ctx, "cannot load cancelled order items to release flash holds",
				slog.String("order_id", order.ID), slog.Any("err", err))
			return
		}
		items = full.Items
	}
	for _, it := range items {
		id := FlashReservationID(order.ID, it.ListingID, it.VariantID)
		if _, err := s.orderFlash.ReleaseFlashSaleStock(ctx, &promotionv1.ReleaseFlashSaleStockRequest{ReservationId: id}); err != nil {
			s.logger.ErrorContext(ctx, "flash hold release failed on cancel",
				slog.String("reservation_id", id), slog.Any("err", err))
		}
	}
}
