package repository

// TEST-STUB (task 1, red): compile-only placeholders so the failing tests build.
// The implementer deletes this file and adds the real definitions to payment.go.

import (
	"context"
	"errors"
)

const PaymentStatusCancelled PaymentStatus = 6

// CancelUnpaidOutcome reports what CancelUnpaid found for the order.
type CancelUnpaidOutcome int

const (
	CancelAbsent CancelUnpaidOutcome = iota
	CancelVoided
	CancelAlreadyCancelled
	CancelOtherStatus
)

func (r *InMemoryPaymentRepository) CancelUnpaid(_ context.Context, _ string) (CancelUnpaidOutcome, error) {
	return CancelAbsent, errors.New("teststub: CancelUnpaid not implemented")
}

func (r *PostgresPaymentRepository) CancelUnpaid(_ context.Context, _ string) (CancelUnpaidOutcome, error) {
	return CancelAbsent, errors.New("teststub: CancelUnpaid not implemented")
}
