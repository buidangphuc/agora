package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"google.golang.org/grpc"

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
