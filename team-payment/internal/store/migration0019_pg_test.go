package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func migration0019Files(t *testing.T) (up, down string) {
	t.Helper()
	ups := migrationFiles(t, 19, 19)
	if len(ups) != 1 {
		t.Fatalf("expected exactly one 0019 up migration, found %d", len(ups))
	}
	up = ups[0]
	down = up[:len(up)-len(".up.sql")] + ".down.sql"
	if m, _ := filepath.Glob(down); len(m) != 1 {
		t.Fatalf("missing %s", down)
	}
	return up, down
}

func pool0019(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := migratedPool(t)
	applyMigration0018(t, pool)
	up, _ := migration0019Files(t)
	applyReset(t, pool, up)
	return pool
}

func TestMigration0019_StoreRefusesSecondCommissionForOnePayment(t *testing.T) {
	// scenario: The store refuses a second commission for one payment
	pool := pool0019(t)
	if err := tryInsert(pool, "c1", "COMMISSION", -14000, "COMPLETED", "pay-1"); err != nil {
		t.Fatalf("valid commission refused: %v", err)
	}
	mustExec(t, pool, insertLedgerSQL, "o1", "seller-fee", "ORDER_SETTLEMENT", int64(310000), "COMPLETED", "pay-1")
	before := scalar[int64](t, pool, `SELECT COALESCE(SUM(amount),0) FROM wallet_ledger WHERE seller_id='seller-fee'`)
	wantCode(t, pool, "23505", "second commission for one payment", insertLedgerSQL, "c2", "seller-fee", "COMMISSION", int64(-100), "COMPLETED", "pay-1")
	if after := scalar[int64](t, pool, `SELECT COALESCE(SUM(amount),0) FROM wallet_ledger WHERE seller_id='seller-fee'`); after != before {
		t.Fatalf("balance changed from %d to %d", before, after)
	}
}

func TestMigration0019_StoreRefusesSecondReversalForOneRefund(t *testing.T) {
	// scenario: The store refuses a second commission for one payment
	pool := pool0019(t)
	if err := tryInsert(pool, "v1", "COMMISSION_REVERSAL", 5600, "COMPLETED", "rpc:r1"); err != nil {
		t.Fatalf("valid reversal refused: %v", err)
	}
	wantCode(t, pool, "23505", "second reversal for one refund", insertLedgerSQL, "v2", "seller-fee", "COMMISSION_REVERSAL", int64(100), "COMPLETED", "rpc:r1")
}

func TestMigration0019_StoreRefusesCommissionThatIsNotNegative(t *testing.T) {
	// scenario: The store refuses a commission that is not negative
	pool := pool0019(t)
	wantCode(t, pool, "23514", "positive commission", insertLedgerSQL, "c-pos", "seller-fee", "COMMISSION", int64(1), "COMPLETED", "fresh-1")
	wantCode(t, pool, "23514", "zero commission", insertLedgerSQL, "c-zero", "seller-fee", "COMMISSION", int64(0), "COMPLETED", "fresh-2")
}

func TestMigration0019_StoreRefusesReversalThatIsNotPositive(t *testing.T) {
	// scenario: The store refuses a reversal that is not positive
	pool := pool0019(t)
	wantCode(t, pool, "23514", "negative reversal", insertLedgerSQL, "v-neg", "seller-fee", "COMMISSION_REVERSAL", int64(-1), "COMPLETED", "fresh-1")
	wantCode(t, pool, "23514", "zero reversal", insertLedgerSQL, "v-zero", "seller-fee", "COMMISSION_REVERSAL", int64(0), "COMPLETED", "fresh-2")
}

func TestMigration0019_StoreRefusesCommissionWithoutReference(t *testing.T) {
	// scenario: The store refuses a commission without a reference
	pool := pool0019(t)
	wantCode(t, pool, "23514", "commission without reference", insertLedgerSQL, "c-noref", "seller-fee", "COMMISSION", int64(-1), "COMPLETED", nil)
	wantCode(t, pool, "23514", "reversal without reference", insertLedgerSQL, "v-noref", "seller-fee", "COMMISSION_REVERSAL", int64(1), "COMPLETED", nil)
}

func TestMigration0019_RateColumnIsNullableAndKeepsExistingTypes(t *testing.T) {
	// scenario: The migration keeps existing ledger rows valid
	pool := pool0019(t)
	mustExec(t, pool, `INSERT INTO wallet_ledger (id, seller_id, type, amount, status, reference_id, commission_rate_bps)
		VALUES ('c-rate', 's', 'COMMISSION', -14000, 'COMPLETED', 'pay-rate', 500)`)
	mustExec(t, pool, insertLedgerSQL, "o-no-rate", "s", "ORDER_SETTLEMENT", int64(1), "COMPLETED", "pay-x")
	if n := scalar[int](t, pool, `SELECT count(*) FROM wallet_ledger WHERE commission_rate_bps IS NULL AND type = 'ORDER_SETTLEMENT'`); n != 1 {
		t.Fatalf("ORDER_SETTLEMENT rows with a null rate = %d, want 1", n)
	}
	if err := tryInsert(pool, "fee-after", "SUBSCRIPTION_FEE", -500, "COMPLETED", "sub-after"); err != nil {
		t.Fatalf("SUBSCRIPTION_FEE arm lost: %v", err)
	}
	if err := tryInsert(pool, "fee-pos", "SUBSCRIPTION_FEE", 500, "COMPLETED", "sub-pos"); err == nil {
		t.Fatal("positive SUBSCRIPTION_FEE accepted")
	}
	if err := tryInsert(pool, "ad-1", "AD_SPEND", -100, "COMPLETED", "ad-ref"); err == nil {
		t.Fatal("AD_SPEND accepted")
	}
}

func TestMigration0019_ExistingLedgerRowsSurviveUpDownUp(t *testing.T) {
	// scenario: The migration keeps existing ledger rows valid
	pool := migratedPool(t)
	applyMigration0018(t, pool)
	for _, r := range []struct {
		id, seller, typ string
		amount          int64
		status          string
		ref             any
	}{
		{"l1", "sa", "ORDER_SETTLEMENT", 1000, "COMPLETED", "tx-1"},
		{"l2", "sa", "REFUND_DEDUCTION", -200, "COMPLETED", "tx-1:r1"},
		{"l3", "sa", "PAYOUT", -300, "PENDING", nil},
		{"l4", "sa", "PAYOUT", 300, "REJECTED", nil},
		{"l5", "sb", "ORDER_SETTLEMENT", 5000, "COMPLETED", "tx-2"},
		{"l6", "sb", "SUBSCRIPTION_FEE", -700, "COMPLETED", "sub-1"},
	} {
		mustExec(t, pool, insertLedgerSQL, r.id, r.seller, r.typ, r.amount, r.status, r.ref)
	}
	sums := func() map[string]int64 {
		out := map[string]int64{}
		rows, err := pool.Query(context.Background(), `SELECT seller_id, SUM(amount)::BIGINT FROM wallet_ledger GROUP BY seller_id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var s string
			var n int64
			if err := rows.Scan(&s, &n); err != nil {
				t.Fatal(err)
			}
			out[s] = n
		}
		return out
	}
	want := sums()
	up, down := migration0019Files(t)
	for i := 0; i < 2; i++ {
		applyReset(t, pool, up)
		if got := sums(); got["sa"] != want["sa"] || got["sb"] != want["sb"] {
			t.Fatalf("after up #%d sums %v, want %v", i+1, got, want)
		}
		if err := tryInsert(pool, "cm", "COMMISSION", -1, "COMPLETED", "cm-ref"); err != nil {
			t.Fatalf("commission refused after up #%d: %v", i+1, err)
		}
		mustExec(t, pool, `DELETE FROM wallet_ledger WHERE id = 'cm'`)
		applyReset(t, pool, down)
		if got := sums(); got["sa"] != want["sa"] || got["sb"] != want["sb"] {
			t.Fatalf("after down #%d sums %v, want %v", i+1, got, want)
		}
		if err := tryInsert(pool, "cm-down", "COMMISSION", -1, "COMPLETED", "cm-ref2"); err == nil {
			t.Fatalf("COMMISSION accepted after down #%d", i+1)
		}
	}
	applyReset(t, pool, up)
}

func TestMigration0019_DownRemovesCommissionRowsSoTheOldCheckHolds(t *testing.T) {
	// scenario: The migration keeps existing ledger rows valid
	pool := pool0019(t)
	mustExec(t, pool, insertLedgerSQL, "o", "s", "ORDER_SETTLEMENT", int64(1000), "COMPLETED", "tx")
	mustExec(t, pool, insertLedgerSQL, "c", "s", "COMMISSION", int64(-50), "COMPLETED", "tx")
	mustExec(t, pool, insertLedgerSQL, "v", "s", "COMMISSION_REVERSAL", int64(20), "COMPLETED", "rpc:r")
	_, down := migration0019Files(t)
	applyReset(t, pool, down)
	if n := scalar[int](t, pool, `SELECT count(*) FROM wallet_ledger WHERE type IN ('COMMISSION','COMMISSION_REVERSAL')`); n != 0 {
		t.Fatalf("%d commission rows left after down", n)
	}
	if n := scalar[int](t, pool, `SELECT count(*) FROM wallet_ledger WHERE type = 'ORDER_SETTLEMENT'`); n != 1 {
		t.Fatalf("ORDER_SETTLEMENT rows = %d, want 1", n)
	}
}
