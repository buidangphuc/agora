package payment

import (
	"context"
	"errors"

	"github.com/buidangphuc/team-payment/internal/store"
)

// ChargeSellerFee debits a subscription fee from the seller's wallet, once per reference.
// A replay returns the existing entry with created=false. It fails with
// ErrInsufficientBalance or *store.FundsOnHoldError when the fee exceeds the balance or
// the withdrawable amount, and ErrFeeReferenceConflict when the reference was booked
// with another amount.
func (s *Service) ChargeSellerFee(ctx context.Context, sellerID string, amount int64, referenceID string) (store.LedgerEntry, bool, error) {
	if sellerID == "" {
		return store.LedgerEntry{}, false, errors.New("seller id is required")
	}
	if amount <= 0 {
		return store.LedgerEntry{}, false, store.ErrInvalidAmount
	}
	if referenceID == "" {
		return store.LedgerEntry{}, false, store.ErrReferenceRequired
	}
	if s.ledgerRepo == nil {
		return store.LedgerEntry{}, false, ErrLedgerNotConfigured
	}
	return s.ledgerRepo.AppendDebitOnce(ctx, store.LedgerEntry{
		SellerID:    sellerID,
		Type:        store.LedgerTypeSubscriptionFee,
		Amount:      -amount,
		Status:      store.LedgerStatusCompleted,
		ReferenceID: referenceID,
	}, store.Holdback{Window: s.holdWindow, Now: s.now()})
}
