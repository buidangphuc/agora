package store_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-payment/internal/store"
)

const (
	payoutPending    = 1
	payoutCompleted  = 3
	payoutRejected   = 4
	obligationWindow = 7 * 24 * time.Hour
)

func ledgerRow(t *testing.T, pool *pgxpool.Pool, id, seller, typ string, amount int64, status, ref string, age time.Duration) {
	t.Helper()
	mustExec(t, pool,
		`INSERT INTO wallet_ledger (id, seller_id, type, amount, status, reference_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), now() - $7::interval)`,
		id, seller, typ, amount, status, ref, fmt.Sprintf("%d seconds", int(age.Seconds())))
}

func payoutRow(t *testing.T, pool *pgxpool.Pool, id, seller string, amount int64, ledgerStatus string, payoutStatus int) {
	t.Helper()
	ledgerRow(t, pool, "l-"+id, seller, store.LedgerTypePayout, -amount, ledgerStatus, "", time.Hour)
	mustExec(t, pool,
		`INSERT INTO payout_requests (id, seller_id, amount, bank_code, account_number, account_name, status, ledger_entry_id)
		 VALUES ($1, $2, $3, 'VCB', '0123456789', 'NGUYEN VAN A', $4, $5)`,
		id, seller, amount, payoutStatus, "l-"+id)
}

func obligations(t *testing.T, pool *pgxpool.Pool, user string) store.PayoutObligations {
	t.Helper()
	got, err := store.NewPostgresLedgerRepository(pool).PayoutObligations(context.Background(), user,
		store.Holdback{Window: obligationWindow, Now: time.Now()})
	if err != nil {
		t.Fatalf("PayoutObligations(%s): %v", user, err)
	}
	return got
}

func TestPayoutObligationsBalances(t *testing.T) {
	t.Run("PAY-1 balance, hold and pending payouts are reported", func(t *testing.T) {
		pool := migratedPool(t)
		ledgerRow(t, pool, "c-old", "S", store.LedgerTypeOrderSettlement, 300000, store.LedgerStatusCompleted, "pay-old", 30*24*time.Hour)
		ledgerRow(t, pool, "c-new", "S", store.LedgerTypeOrderSettlement, 200000, store.LedgerStatusCompleted, "pay-new", time.Hour)
		payoutRow(t, pool, "p-1", "S", 50000, store.LedgerStatusPending, payoutPending)
		payoutRow(t, pool, "p-2", "S", 50000, store.LedgerStatusPending, payoutPending)
		got := obligations(t, pool, "S")
		if got.Available != 200000 || got.Held != 200000 || got.Pending != 2 {
			t.Fatalf("PAY-1 got %+v, want available 200000 held 200000 pending 2", got)
		}
	})

	t.Run("PAY-2 a user with no wallet reads zeros", func(t *testing.T) {
		pool := migratedPool(t)
		got := obligations(t, pool, "nobody")
		if got.Available != 0 || got.Held != 0 || got.Pending != 0 {
			t.Fatalf("PAY-2 got %+v, want zeros", got)
		}
	})

	t.Run("PAY-3 completed and rejected payouts are not pending", func(t *testing.T) {
		pool := migratedPool(t)
		ledgerRow(t, pool, "c-1", "S", store.LedgerTypeOrderSettlement, 500000, store.LedgerStatusCompleted, "pay-1", 30*24*time.Hour)
		payoutRow(t, pool, "p-1", "S", 50000, store.LedgerStatusCompleted, payoutCompleted)
		payoutRow(t, pool, "p-2", "S", 50000, store.LedgerStatusRejected, payoutRejected)
		if got := obligations(t, pool, "S"); got.Pending != 0 {
			t.Fatalf("PAY-3 pending = %d, want 0", got.Pending)
		}
	})

	t.Run("PAY-4 a negative balance reads as no available money", func(t *testing.T) {
		pool := migratedPool(t)
		ledgerRow(t, pool, "d-1", "S", store.LedgerTypeRefundDeduction, -50000, store.LedgerStatusCompleted, "rf-1", time.Hour)
		got := obligations(t, pool, "S")
		if got.Available != 0 || got.Held != 0 {
			t.Fatalf("PAY-4 got %+v, want available 0 held 0", got)
		}
	})

	t.Run("PAY-5 another seller's money is not included", func(t *testing.T) {
		pool := migratedPool(t)
		ledgerRow(t, pool, "c-s", "S", store.LedgerTypeOrderSettlement, 100000, store.LedgerStatusCompleted, "pay-s", 30*24*time.Hour)
		ledgerRow(t, pool, "c-t", "T", store.LedgerTypeOrderSettlement, 900000, store.LedgerStatusCompleted, "pay-t", time.Hour)
		payoutRow(t, pool, "p-t", "T", 50000, store.LedgerStatusPending, payoutPending)
		got := obligations(t, pool, "S")
		if got.Available != 100000 || got.Held != 0 || got.Pending != 0 {
			t.Fatalf("PAY-5 got %+v, want available 100000 held 0 pending 0", got)
		}
	})
}
