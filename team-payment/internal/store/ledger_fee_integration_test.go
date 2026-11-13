package store_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const insertLedgerSQL = `INSERT INTO wallet_ledger (id, seller_id, type, amount, status, reference_id)
	VALUES ($1, $2, $3, $4, $5, $6)`

func applyMigration0018(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	files := migrationFiles(t, 18, 18)
	if len(files) != 1 {
		t.Fatalf("expected exactly one 0018 up migration, found %d", len(files))
	}
	applyReset(t, pool, files[0])
}

func tryInsert(pool *pgxpool.Pool, id, typ string, amount int64, status string, ref any) error {
	_, err := pool.Exec(context.Background(), insertLedgerSQL, id, "seller-fee", typ, amount, status, ref)
	return err
}

// seller-fee-charge req5: The store refuses a positive fee row; refuses a fee row without a
// reference; refuses a second fee row for one reference.
func TestSubscriptionFeeConstraints(t *testing.T) {
	pool := migratedPool(t)
	applyMigration0018(t, pool)

	t.Run("accepts a negative referenced fee row", func(t *testing.T) {
		if err := tryInsert(pool, "fee-ok", "SUBSCRIPTION_FEE", -500, "COMPLETED", "sub-ok"); err != nil {
			t.Fatalf("valid fee row refused: %v", err)
		}
	})

	t.Run("refuses a positive fee row", func(t *testing.T) {
		if err := tryInsert(pool, "fee-pos", "SUBSCRIPTION_FEE", 500, "COMPLETED", "sub-pos"); err == nil {
			t.Fatal("positive fee row accepted")
		}
	})

	t.Run("refuses a zero fee row", func(t *testing.T) {
		if err := tryInsert(pool, "fee-zero", "SUBSCRIPTION_FEE", 0, "COMPLETED", "sub-zero"); err == nil {
			t.Fatal("zero fee row accepted")
		}
	})

	t.Run("refuses a fee row without a reference", func(t *testing.T) {
		if err := tryInsert(pool, "fee-noref", "SUBSCRIPTION_FEE", -500, "COMPLETED", nil); err == nil {
			t.Fatal("fee row without reference accepted")
		}
	})

	t.Run("refuses a second fee row for one reference", func(t *testing.T) {
		if err := tryInsert(pool, "fee-dup", "SUBSCRIPTION_FEE", -500, "COMPLETED", "sub-ok"); err == nil {
			t.Fatal("second fee row for the same reference accepted")
		}
		if n := scalar[int](t, pool, `SELECT count(*) FROM wallet_ledger WHERE type='SUBSCRIPTION_FEE' AND reference_id='sub-ok'`); n != 1 {
			t.Fatalf("fee rows for reference = %d, want 1", n)
		}
	})

	t.Run("ad spend stays refused", func(t *testing.T) {
		if err := tryInsert(pool, "ad-1", "AD_SPEND", -100, "COMPLETED", "ad-ref"); err == nil {
			t.Fatal("AD_SPEND row accepted")
		}
	})
}

// seller-fee-charge req5: Existing ledger types still migrate cleanly.
func TestSubscriptionFeeMigration_ExistingTypesStillMigrate(t *testing.T) {
	pool := migratedPool(t)
	seed := []struct {
		id, typ string
		amount  int64
		status  string
		ref     any
	}{
		{"l-settle", "ORDER_SETTLEMENT", 1000, "COMPLETED", "tx-1"},
		{"l-refund", "REFUND_DEDUCTION", -200, "COMPLETED", "tx-1:r1"},
		{"l-payout", "PAYOUT", -300, "PENDING", nil},
		{"l-payout-rej", "PAYOUT", 300, "REJECTED", nil},
	}
	for _, s := range seed {
		if err := tryInsert(pool, s.id, s.typ, s.amount, s.status, s.ref); err != nil {
			t.Fatalf("seed %s: %v", s.id, err)
		}
	}

	applyMigration0018(t, pool)

	if n := scalar[int](t, pool, `SELECT count(*) FROM wallet_ledger`); n != len(seed) {
		t.Fatalf("rows after migration = %d, want %d", n, len(seed))
	}
	if sum := scalar[int64](t, pool, `SELECT COALESCE(SUM(amount),0)::bigint FROM wallet_ledger`); sum != 800 {
		t.Fatalf("ledger sum after migration = %d, want 800", sum)
	}

	if err := tryInsert(pool, "n-settle-neg", "ORDER_SETTLEMENT", -1, "COMPLETED", "tx-2"); err == nil {
		t.Fatal("negative settlement accepted after migration")
	}
	if err := tryInsert(pool, "n-refund-pos", "REFUND_DEDUCTION", 1, "COMPLETED", "tx-3"); err == nil {
		t.Fatal("positive refund deduction accepted after migration")
	}
	if err := tryInsert(pool, "n-settle-noref", "ORDER_SETTLEMENT", 1, "COMPLETED", nil); err == nil {
		t.Fatal("settlement without reference accepted after migration")
	}
	if err := tryInsert(pool, "n-settle-dup", "ORDER_SETTLEMENT", 5, "COMPLETED", "tx-1"); err == nil {
		t.Fatal("duplicate settlement reference accepted after migration")
	}
	if err := tryInsert(pool, "ok-settle", "ORDER_SETTLEMENT", 5, "COMPLETED", "tx-9"); err != nil {
		t.Fatalf("valid settlement refused after migration: %v", err)
	}
}
