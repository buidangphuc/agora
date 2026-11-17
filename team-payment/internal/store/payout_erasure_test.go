package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-payment/internal/store"
)

const mask = "Đã xóa"

func snapshotMoney(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var s string
	err := pool.QueryRow(context.Background(), `SELECT COALESCE(string_agg(x, '|' ORDER BY x), '') FROM (
		SELECT row_to_json(p)::text AS x FROM payout_requests p
		UNION ALL SELECT row_to_json(l)::text FROM wallet_ledger l
		UNION ALL SELECT row_to_json(t)::text FROM payment_transactions t) q`).Scan(&s)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func seedSeller(t *testing.T, pool *pgxpool.Pool, seller string) {
	t.Helper()
	ledgerRow(t, pool, "c-"+seller, seller, store.LedgerTypeOrderSettlement, 500000, store.LedgerStatusCompleted, "pay-"+seller, 30*24*time.Hour)
	payoutRow(t, pool, "p-"+seller+"-1", seller, 50000, store.LedgerStatusPending, payoutPending)
	payoutRow(t, pool, "p-"+seller+"-2", seller, 30000, store.LedgerStatusCompleted, payoutCompleted)
	payoutRow(t, pool, "p-"+seller+"-3", seller, 20000, store.LedgerStatusRejected, payoutRejected)
}

func masked(t *testing.T, pool *pgxpool.Pool, seller string) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM payout_requests WHERE seller_id = $1 AND account_number = $2 AND account_name = $2`, seller, mask).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEraseSellerPayouts(t *testing.T) {
	t.Run("PER-1 bank details are masked and money records are kept", func(t *testing.T) {
		pool := migratedPool(t)
		seedSeller(t, pool, "S")
		before, err := store.NewPostgresWalletRepository(pool).ListPayoutRequestsBySellerID(t.Context(), "S")
		if err != nil || len(before) != 3 {
			t.Fatalf("seed list = %d, %v", len(before), err)
		}
		ledgerBefore, _ := store.NewPostgresLedgerRepository(pool).Balance(t.Context(), "S")

		if err := store.NewPostgresWalletRepository(pool).EraseSellerPayouts(t.Context(), "S"); err != nil {
			t.Fatalf("PER-1 EraseSellerPayouts: %v", err)
		}
		after, err := store.NewPostgresWalletRepository(pool).ListPayoutRequestsBySellerID(t.Context(), "S")
		if err != nil || len(after) != 3 {
			t.Fatalf("PER-1 list after = %d, %v", len(after), err)
		}
		byID := map[string]store.PayoutRequest{}
		for _, p := range before {
			byID[p.ID] = p
		}
		for _, a := range after {
			b := byID[a.ID]
			if a.AccountNumber != mask || a.AccountName != mask {
				t.Errorf("PER-1 %s not masked: %q / %q", a.ID, a.AccountNumber, a.AccountName)
			}
			if a.BankCode != b.BankCode || a.Amount != b.Amount || a.Status != b.Status || a.LedgerEntryID != b.LedgerEntryID || a.SellerID != b.SellerID {
				t.Errorf("PER-1 %s money fields changed: %+v -> %+v", a.ID, b, a)
			}
		}
		if got, _ := store.NewPostgresLedgerRepository(pool).Balance(t.Context(), "S"); got != ledgerBefore {
			t.Errorf("PER-1 ledger sum %d -> %d", ledgerBefore, got)
		}
	})

	t.Run("PER-2 another seller's payouts are untouched", func(t *testing.T) {
		pool := migratedPool(t)
		seedSeller(t, pool, "S")
		seedSeller(t, pool, "T")
		if err := store.NewPostgresWalletRepository(pool).EraseSellerPayouts(t.Context(), "S"); err != nil {
			t.Fatal(err)
		}
		if masked(t, pool, "S") != 3 || masked(t, pool, "T") != 0 {
			t.Fatalf("PER-2 masked S=%d T=%d, want 3/0", masked(t, pool, "S"), masked(t, pool, "T"))
		}
		var n int
		if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM payout_requests WHERE seller_id='T' AND account_number='0123456789' AND account_name='NGUYEN VAN A'`).Scan(&n); err != nil || n != 3 {
			t.Fatalf("PER-2 T rows intact = %d, %v", n, err)
		}
	})

	t.Run("PER-3 erasing twice changes nothing", func(t *testing.T) {
		pool := migratedPool(t)
		seedSeller(t, pool, "S")
		w := store.NewPostgresWalletRepository(pool)
		if err := w.EraseSellerPayouts(t.Context(), "S"); err != nil {
			t.Fatal(err)
		}
		first := snapshotMoney(t, pool)
		for i := 0; i < 2; i++ {
			if err := w.EraseSellerPayouts(t.Context(), "S"); err != nil {
				t.Fatalf("PER-3 repeat %d: %v", i, err)
			}
		}
		if again := snapshotMoney(t, pool); again != first {
			t.Fatalf("PER-3 rows differ after repeat\nfirst: %s\nagain: %s", first, again)
		}
	})

	t.Run("PER-4 a user without payouts is a no-op", func(t *testing.T) {
		pool := migratedPool(t)
		seedSeller(t, pool, "S")
		if _, err := store.NewPostgresPaymentRepository(pool).CreateTransaction(t.Context(), store.PaymentTransaction{OrderID: "o-1", BuyerID: "B", Amount: 1000}); err != nil {
			t.Fatal(err)
		}
		before := snapshotMoney(t, pool)
		if err := store.NewPostgresWalletRepository(pool).EraseSellerPayouts(t.Context(), "B"); err != nil {
			t.Fatalf("PER-4 EraseSellerPayouts: %v", err)
		}
		if after := snapshotMoney(t, pool); after != before {
			t.Fatalf("PER-4 rows changed\nbefore: %s\nafter:  %s", before, after)
		}
	})
}

func TestEraseSellerPayoutsRollsBack(t *testing.T) {
	// PER-6
	ctx := context.Background()
	pool := migratedPool(t)
	seedSeller(t, pool, "S")
	mustExec(t, pool, `CREATE FUNCTION fail_payout_erase() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected failure'; END $$`)
	mustExec(t, pool, `CREATE TRIGGER fail_payout_erase BEFORE UPDATE ON payout_requests FOR EACH ROW WHEN (OLD.id = 'p-S-3') EXECUTE FUNCTION fail_payout_erase()`)
	before := snapshotMoney(t, pool)

	w := store.NewPostgresWalletRepository(pool)
	if err := w.EraseSellerPayouts(ctx, "S"); err == nil {
		t.Fatal("PER-6 expected an error from the injected failure")
	}
	if n := masked(t, pool, "S"); n != 0 {
		t.Fatalf("PER-6 %d payouts masked after a failed erase, want 0", n)
	}
	if after := snapshotMoney(t, pool); after != before {
		t.Fatal("PER-6 rows changed after a failed erase")
	}

	mustExec(t, pool, `DROP TRIGGER fail_payout_erase ON payout_requests`)
	if err := w.EraseSellerPayouts(ctx, "S"); err != nil {
		t.Fatalf("PER-6 retry: %v", err)
	}
	if n := masked(t, pool, "S"); n != 3 {
		t.Fatalf("PER-6 masked after recovery = %d, want 3", n)
	}
}
