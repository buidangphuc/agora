package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"google.golang.org/grpc"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
)

// PromotionClient is the slice of team-promotion's VoucherService the checkout
// saga needs. It is an interface (not the concrete generated client) so tests can
// fake it — mirroring how upstream.DomainClient is faked for the stock service.
// The generated promotionv1.VoucherServiceClient satisfies it directly.
type PromotionClient interface {
	ValidateAndReserve(ctx context.Context, in *promotionv1.ValidateAndReserveRequest, opts ...grpc.CallOption) (*promotionv1.ValidateAndReserveResponse, error)
	CommitReservation(ctx context.Context, in *promotionv1.CommitReservationRequest, opts ...grpc.CallOption) (*promotionv1.CommitReservationResponse, error)
	ReleaseReservation(ctx context.Context, in *promotionv1.ReleaseReservationRequest, opts ...grpc.CallOption) (*promotionv1.ReleaseReservationResponse, error)
}

// ErrVoucherRejected is returned when the promotion service declines a voucher
// (expired, below min-spend, quota exhausted). The handler maps it to
// codes.FailedPrecondition with the promotion-supplied reason — an invalid voucher
// is never silently dropped, and the order is not created at full price.
var ErrVoucherRejected = errors.New("voucher rejected")

// WithPromotionClient wires the voucher redemption client. When it is left unset
// (nil), CreateOrdersFromCart ignores voucher_code entirely, so the existing
// no-voucher checkout flow is unchanged (degrade-open on an unconfigured or
// unreachable promotion service).
func WithPromotionClient(pc PromotionClient) OrderServiceOption {
	return func(s *OrderService) {
		if pc != nil {
			s.promo = pc
		}
	}
}

// reserveVoucher validates a voucher and places a discount hold for this checkout,
// keyed by the caller-owned reservationID. ValidateAndReserve is idempotent on that
// id (same id → same discount, quota decremented once). A declined voucher returns
// ErrVoucherRejected wrapping the promotion reason. Returns the discount in minor
// units (never negative).
func (s *OrderService) reserveVoucher(ctx context.Context, reservationID, code, buyerID string, cartSubtotal int64, sellerID string) (int64, error) {
	resp, err := s.promo.ValidateAndReserve(ctx, &promotionv1.ValidateAndReserveRequest{
		ReservationId: reservationID,
		Code:          code,
		BuyerId:       buyerID,
		CartSubtotal:  cartSubtotal,
		SellerId:      sellerID,
	})
	if err != nil {
		return 0, fmt.Errorf("validate voucher %q: %w", code, err)
	}
	if !resp.GetValid() {
		reason := resp.GetReason()
		if reason == "" {
			reason = "voucher is not valid"
		}
		return 0, fmt.Errorf("%w: %s", ErrVoucherRejected, reason)
	}
	discount := resp.GetDiscountAmount()
	if discount < 0 {
		discount = 0
	}
	return discount, nil
}

// releaseVoucher returns a discount hold to the promotion service on saga
// failure/compensation. It is best-effort and idempotent on reservationID (a
// released or unknown id is a no-op there); a failure is logged, never fatal, and
// runs on the caller-supplied (background) context so a cancelled request context
// cannot abort it. A blank id or no promotion client is a no-op.
func (s *OrderService) releaseVoucher(ctx context.Context, reservationID string) {
	if s.promo == nil || reservationID == "" {
		return
	}
	if _, err := s.promo.ReleaseReservation(ctx, &promotionv1.ReleaseReservationRequest{ReservationId: reservationID}); err != nil {
		s.logger.WarnContext(ctx, "voucher release failed",
			slog.String("reservation_id", reservationID), slog.Any("err", err))
	}
}

const (
	voucherReasonUnavailable = "vouchers unavailable"
	voucherReasonNotApplies  = "voucher does not apply to this checkout"
	voucherReasonWrongSlot   = "voucher cannot be used in this slot"
)

type shopVoucherSlot struct {
	sellerID string
	code     string
}

type voucherSlots struct {
	legacyCode   string
	platformCode string
	shops        []shopVoucherSlot
}

type voucherResolveInput struct {
	buyerID               string
	groups                []quoteGroupInput
	slots                 voucherSlots
	platformReservationID string
	shopReservationID     func(sellerID string) string
}

type voucherOutcome struct {
	code          string
	valid         bool
	reason        string
	discount      int64
	scope         orderv1.CheckoutVoucherScope
	reservationID string
}

type resolvedVouchers struct {
	shops    map[string]voucherOutcome
	platform voucherOutcome
}

func (s *OrderService) resolveVouchers(ctx context.Context, in voucherResolveInput) (resolvedVouchers, error) {
	out := resolvedVouchers{shops: map[string]voucherOutcome{}}
	platformCode := in.slots.platformCode
	if platformCode == "" {
		platformCode = in.slots.legacyCode
	}
	subtotals := make(map[string]int64, len(in.groups))
	var total int64
	for _, g := range in.groups {
		subtotals[g.sellerID] += g.itemsSubtotal
		total += g.itemsSubtotal
	}

	for _, sl := range in.slots.shops {
		if sl.code == "" {
			continue
		}
		if s.promo == nil {
			out.shops[sl.sellerID] = voucherOutcome{code: sl.code, reason: voucherReasonUnavailable}
			continue
		}
		sub, ok := subtotals[sl.sellerID]
		if !ok {
			out.shops[sl.sellerID] = voucherOutcome{code: sl.code, reason: voucherReasonNotApplies}
			continue
		}
		o, err := s.routeVoucher(ctx, in.buyerID, sl.code, in.shopReservationID(sl.sellerID), sub, sl.sellerID, orderv1.CheckoutVoucherScope_CHECKOUT_VOUCHER_SCOPE_SHOP)
		if err != nil {
			return resolvedVouchers{}, err
		}
		out.shops[sl.sellerID] = o
	}

	if platformCode != "" {
		if s.promo == nil {
			out.platform = voucherOutcome{code: platformCode, reason: voucherReasonUnavailable}
			return out, nil
		}
		o, err := s.routeVoucher(ctx, in.buyerID, platformCode, in.platformReservationID, total, "", orderv1.CheckoutVoucherScope_CHECKOUT_VOUCHER_SCOPE_PLATFORM)
		if err != nil {
			return resolvedVouchers{}, err
		}
		out.platform = o
	}
	return out, nil
}

func (s *OrderService) routeVoucher(ctx context.Context, buyerID, code, reservationID string, subtotal int64, sellerID string, want orderv1.CheckoutVoucherScope) (voucherOutcome, error) {
	o := voucherOutcome{code: code}
	resp, err := s.promo.ValidateAndReserve(ctx, &promotionv1.ValidateAndReserveRequest{
		ReservationId: reservationID,
		Code:          code,
		BuyerId:       buyerID,
		CartSubtotal:  subtotal,
		SellerId:      sellerID,
	})
	if err != nil {
		return o, fmt.Errorf("%w: validate voucher %q: %v", ErrPromotionUnavailable, code, err)
	}
	if !resp.GetValid() {
		o.reason = resp.GetReason()
		if o.reason == "" {
			o.reason = "voucher is not valid"
		}
		return o, nil
	}
	var got orderv1.CheckoutVoucherScope
	switch resp.GetScope() {
	case promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP:
		got = orderv1.CheckoutVoucherScope_CHECKOUT_VOUCHER_SCOPE_SHOP
	case promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM:
		got = orderv1.CheckoutVoucherScope_CHECKOUT_VOUCHER_SCOPE_PLATFORM
	}
	wrongSeller := want == orderv1.CheckoutVoucherScope_CHECKOUT_VOUCHER_SCOPE_SHOP && resp.GetSellerId() != "" && resp.GetSellerId() != sellerID
	if got != want || wrongSeller {
		s.releaseVoucher(ctx, reservationID)
		o.reason = voucherReasonWrongSlot
		return o, nil
	}
	o.valid = true
	o.scope = want
	o.reservationID = reservationID
	o.discount = resp.GetDiscountAmount()
	if o.discount < 0 {
		o.discount = 0
	}
	return o, nil
}
