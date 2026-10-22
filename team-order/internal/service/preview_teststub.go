package service

import (
	"context"
	"errors"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
)

// TEST-STUB for task 6: compile-only placeholder. The implementer deletes this
// file and implements PreviewCheckout in order.go with exactly this signature.
// ErrAddressNotFound ("address not found") and ErrAddressUnavailable (identity
// unreachable or no identity client) move to order.go too.

var (
	ErrAddressNotFound    = errors.New("address not found")
	ErrAddressUnavailable = errors.New("address service unavailable; please retry")
)

func (s *OrderService) PreviewCheckout(ctx context.Context, buyerID, addressID string, itemIDs []string, opts ...CheckoutOption) (*orderv1.PreviewCheckoutResponse, error) {
	return nil, errors.New("PreviewCheckout not implemented")
}
