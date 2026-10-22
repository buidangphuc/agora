package service

// TEST-STUB for task 4 (voucher routing): compile-only; the implementer replaces
// this file by moving these declarations into redemption.go with real behaviour.

import (
	"context"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
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
	return resolvedVouchers{}, nil
}
