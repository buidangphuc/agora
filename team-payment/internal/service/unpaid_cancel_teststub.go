package service

import (
	"context"
	"errors"
)

// TEST-STUB (task 2, compile-only): the implementer deletes this file and adds the real
// ErrPaymentCancelled and VoidCancelledOrder to payment.go / settlement.go.

// ErrPaymentCancelled: the transaction's order was cancelled unpaid; it cannot be paid.
var ErrPaymentCancelled = errors.New("payment cancelled")

// VoidCancelledOrder is a stub that does nothing.
func (s *PaymentService) VoidCancelledOrder(_ context.Context, _ string) error { return nil }
