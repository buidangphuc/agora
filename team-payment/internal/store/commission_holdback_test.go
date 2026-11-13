package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-payment/internal/store"
)

func TestHeldNet_HeldSaleIsHeldNetOfItsCommission(t *testing.T) {
	// scenario: A held sale is held net of its commission
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		now := time.Now()
		creditAt(t, s, "s", "o-old", 300000, now.Add(-window-time.Hour))
		tx := creditAt(t, s, "s", "o-new", 310000, now.Add(-time.Minute))
		if _, created, err := s.ledger.AppendEntryOnce(context.Background(), store.LedgerEntry{
			SellerID: "s", Type: store.LedgerTypeCommission, Amount: -14000,
			Status: store.LedgerStatusCompleted, ReferenceID: tx.ID, CreatedAt: now.Add(-time.Minute),
		}); err != nil || !created {
			t.Fatalf("commission entry: %v %v", created, err)
		}
		if err := payout(s, "s", 300000, now, window); err != nil {
			t.Fatalf("first payout: %v", err)
		}
		if err := payout(s, "s", 1, now, window); !errors.Is(err, store.ErrFundsOnHold) {
			t.Fatalf("second payout: want ErrFundsOnHold, got %v", err)
		}
		if _, bal := ledgerOf(t, s, "s"); bal != 296000 {
			t.Fatalf("balance %d, want 296000", bal)
		}
	})
}

func TestHeldNet_RefundedCommissionedSaleIsHeldNetOfDeductionAndReversal(t *testing.T) {
	// scenario: A held sale is held net of its commission
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		now := time.Now()
		creditAt(t, s, "s", "o-old", 300000, now.Add(-window-time.Hour))
		tx, _ := settle(t, s, "o-new", "s", 310000, withBreakdown(280000, 0, 500))
		if _, err := rpcRefund(context.Background(), s, tx.ID, "r1", 124000); err != nil {
			t.Fatalf("refund: %v", err)
		}
		if err := payout(s, "s", 300000, now.Add(time.Minute), window); err != nil {
			t.Fatalf("first payout: %v", err)
		}
		if err := payout(s, "s", 1, now.Add(time.Minute), window); !errors.Is(err, store.ErrFundsOnHold) {
			t.Fatalf("second payout: want ErrFundsOnHold, got %v", err)
		}
		if _, bal := ledgerOf(t, s, "s"); bal != 177600 {
			t.Fatalf("balance %d, want 177600", bal)
		}
	})
}

func TestHeldNet_PayoutOverTheNetIsRefused(t *testing.T) {
	// scenario: A held sale is held net of its commission
	eachCommissionBackend(t, func(t *testing.T, s stores, _ *pgxpool.Pool) {
		now := time.Now()
		settle(t, s, "o-fresh", "s", 310000, withBreakdown(280000, 0, 500))
		if err := payout(s, "s", 1, now, window); !errors.Is(err, store.ErrFundsOnHold) {
			t.Fatalf("payout of a fresh sale: want ErrFundsOnHold, got %v", err)
		}
		if got, _ := ledgerOf(t, s, "s"); len(got[store.LedgerTypePayout]) != 0 {
			t.Fatalf("payout written: %+v", got[store.LedgerTypePayout])
		}
	})
}
