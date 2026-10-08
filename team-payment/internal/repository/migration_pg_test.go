package repository_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-payment/internal/pgtest"
)

// signPrecheck is the migration plan's pre-check: rows that would violate the 0006 sign
// rule. It must return zero rows before 0006 is applied to a long-lived DB.
const signPrecheck = `SELECT type, count(*) FROM wallet_ledger WHERE NOT (
	(type = 'ORDER_SETTLEMENT' AND amount > 0) OR (type = 'REFUND_DEDUCTION' AND amount < 0)
	OR (type = 'PAYOUT' AND (amount < 0 OR (status = 'REJECTED' AND amount > 0)))) GROUP BY type`

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func wantCode(t *testing.T, pool *pgxpool.Pool, code, what, sql string, args ...any) {
	t.Helper()
	_, err := pool.Exec(context.Background(), sql, args...)
	if got := pgCode(err); got != code {
		t.Fatalf("%s: want SQLSTATE %s, got %v", what, code, err)
	}
}

// 0006 applies on a payment_db that already holds legacy unreferenced credits and a
// REJECTED +amount payout reversal, keeps them readable and counted, enforces the
// uniqueness / sign / reference rules for new rows, and its down migration reverses it.
func TestMigration0006_LegacyDataAndRules_Postgres(t *testing.T) {
	pool := pgtest.EmptyPool(t)
	ctx := context.Background()
	var m0006up, m0006down string
	for _, f := range pgtest.UpFiles(t) {
		if strings.HasPrefix(filepath.Base(f), "0006_") {
			m0006up = f
			m0006down = strings.TrimSuffix(f, ".up.sql") + ".down.sql"
			continue
		}
		pgtest.Apply(t, pool, f)
	}
	if m0006up == "" {
		t.Fatal("migration 0006 not found")
	}

	// Legacy data written by the old binary.
	mustExec(t, pool, `INSERT INTO payment_transactions (id, order_id, buyer_id, amount, status) VALUES ('tx-legacy', 'o-legacy', 'b', 500, 2)`)
	mustExec(t, pool, `INSERT INTO wallet_ledger (id, seller_id, type, amount, status) VALUES
		('l1', 's-legacy', 'ORDER_SETTLEMENT', 500, 'COMPLETED'),
		('l2', 's-legacy', 'PAYOUT', -200, 'PENDING'),
		('l3', 's-legacy', 'PAYOUT', -50, 'PENDING'),
		('l4', 's-legacy', 'PAYOUT', 50, 'REJECTED')`)

	rows, err := pool.Query(ctx, signPrecheck)
	if err != nil {
		t.Fatalf("precheck: %v", err)
	}
	n := 0
	for rows.Next() {
		n++
	}
	rows.Close()
	if n != 0 {
		t.Fatalf("sign precheck returned %d rows, want 0", n)
	}

	pgtest.Apply(t, pool, m0006up)

	var bal int64
	if err := pool.QueryRow(ctx, `SELECT SUM(amount) FROM wallet_ledger WHERE seller_id = 's-legacy'`).Scan(&bal); err != nil || bal != 300 {
		t.Fatalf("legacy balance = %d (%v), want 300", bal, err)
	}
	var refunded int64
	if err := pool.QueryRow(ctx, `SELECT refunded_amount FROM payment_transactions WHERE id = 'tx-legacy'`).Scan(&refunded); err != nil || refunded != 0 {
		t.Fatalf("refunded_amount = %d (%v), want 0", refunded, err)
	}

	mustExec(t, pool, `INSERT INTO wallet_ledger (id, seller_id, type, amount, status, reference_id) VALUES ('c1', 's', 'ORDER_SETTLEMENT', 500, 'COMPLETED', 'tx-1')`)
	wantCode(t, pool, "23505", "second settlement for one payment",
		`INSERT INTO wallet_ledger (id, seller_id, type, amount, status, reference_id) VALUES ('c2', 's', 'ORDER_SETTLEMENT', 500, 'COMPLETED', 'tx-1')`)
	wantCode(t, pool, "23514", "non-positive settlement",
		`INSERT INTO wallet_ledger (id, seller_id, type, amount, status, reference_id) VALUES ('c3', 's', 'ORDER_SETTLEMENT', -1, 'COMPLETED', 'tx-fresh')`)
	wantCode(t, pool, "23514", "refund deduction without reference",
		`INSERT INTO wallet_ledger (id, seller_id, type, amount, status) VALUES ('c4', 's', 'REFUND_DEDUCTION', -1, 'COMPLETED')`)
	wantCode(t, pool, "23514", "new settlement without reference",
		`INSERT INTO wallet_ledger (id, seller_id, type, amount, status) VALUES ('c5', 's', 'ORDER_SETTLEMENT', 1, 'COMPLETED')`)
	wantCode(t, pool, "23514", "non-negative refund deduction",
		`INSERT INTO wallet_ledger (id, seller_id, type, amount, status, reference_id) VALUES ('c6', 's', 'REFUND_DEDUCTION', 1, 'COMPLETED', 'tx-1')`)
	wantCode(t, pool, "23514", "positive non-rejected payout",
		`INSERT INTO wallet_ledger (id, seller_id, type, amount, status) VALUES ('c7', 's', 'PAYOUT', 1, 'PENDING')`)
	wantCode(t, pool, "23514", "unknown type",
		`INSERT INTO wallet_ledger (id, seller_id, type, amount, status) VALUES ('c8', 's', 'BONUS', 1, 'COMPLETED')`)
	mustExec(t, pool, `INSERT INTO wallet_ledger (id, seller_id, type, amount, status) VALUES ('c9', 's', 'PAYOUT', 10, 'REJECTED')`)
	mustExec(t, pool, `INSERT INTO wallet_ledger (id, seller_id, type, amount, status, reference_id) VALUES ('c10', 's', 'REFUND_DEDUCTION', -100, 'COMPLETED', 'tx-1')`)
	wantCode(t, pool, "23514", "refunded_amount above amount",
		`UPDATE payment_transactions SET refunded_amount = 501 WHERE id = 'tx-legacy'`)
	wantCode(t, pool, "23514", "negative refunded_amount",
		`UPDATE payment_transactions SET refunded_amount = -1 WHERE id = 'tx-legacy'`)

	// Down reverses it; up applies again over the same data.
	pgtest.Apply(t, pool, m0006down)
	var cols int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND (table_name, column_name) IN
		(('wallet_ledger', 'reference_id'), ('payment_transactions', 'refunded_amount'))`).Scan(&cols); err != nil || cols != 0 {
		t.Fatalf("after down: %d columns left (%v), want 0", cols, err)
	}
	mustExec(t, pool, `INSERT INTO wallet_ledger (id, seller_id, type, amount, status) VALUES ('d1', 's', 'ORDER_SETTLEMENT', -5, 'COMPLETED')`)
	mustExec(t, pool, `DELETE FROM wallet_ledger WHERE id = 'd1'`)
	pgtest.Apply(t, pool, m0006up)
}
