package repository_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-payment/internal/pgtest"
)

// precheck0007 is design D9's pre-check: both queries must return zero rows before 0007
// is applied to a long-lived DB.
var precheck0007 = []string{
	`SELECT id FROM payment_transactions WHERE status = 5`,
	`SELECT d.id FROM wallet_ledger d
	WHERE d.type = 'REFUND_DEDUCTION'
	  AND d.reference_id IS NOT NULL
	  AND NOT EXISTS (SELECT 1 FROM payment_transactions t WHERE t.id = d.reference_id)`,
}

func countRows(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	rows, err := pool.Query(context.Background(), sql, args...)
	if err != nil {
		t.Fatalf("query %q: %v", sql, err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows %q: %v", sql, err)
	}
	return n
}

func scalar[T any](t *testing.T, pool *pgxpool.Pool, sql string, args ...any) T {
	t.Helper()
	var v T
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&v); err != nil {
		t.Fatalf("scalar %q: %v", sql, err)
	}
	return v
}

// migration0007Files applies every migration before 0007 and returns 0007's up/down.
func migration0007Files(t *testing.T, pool *pgxpool.Pool) (up, down string) {
	t.Helper()
	for _, f := range pgtest.UpFiles(t) {
		base := filepath.Base(f)
		switch {
		case strings.HasPrefix(base, "0007_"):
			up, down = f, strings.TrimSuffix(f, ".up.sql")+".down.sql"
		case base < "0007_":
			pgtest.Apply(t, pool, f)
		}
	}
	if up == "" {
		t.Fatal("migration 0007 not found")
	}
	return up, down
}

// applyReset applies a migration and drops the pooled connections, whose cached
// statement plans would otherwise refer to the column types before it.
func applyReset(t *testing.T, pool *pgxpool.Pool, file string) {
	t.Helper()
	pgtest.Apply(t, pool, file)
	pool.Reset()
}

// 0007 on a database at 0006 holding the four kinds of payment the old model produced
// (payment-cumulative-refunds: "Payments refunded under the single-refund model keep their
// outcome after the upgrade").
func TestMigration0007_LegacyRefunds_Postgres(t *testing.T) {
	pool := pgtest.EmptyPool(t)
	up, down := migration0007Files(t, pool)

	// Written by the 0006 binary.
	mustExec(t, pool, `INSERT INTO payment_transactions (id, order_id, buyer_id, amount, status, provider_reference, refunded_amount) VALUES
		('tx-partial', 'o-partial', 'b', 500000, 4, 'REFUND:damaged', 200000),
		('tx-full',    'o-full',    'b', 300000, 4, 'REFUND:order_cancelled', 300000),
		('tx-pre6',    'o-pre6',    'b', 400000, 4, 'REFUND:old', 0),
		('tx-paid',    'o-paid',    'b', 100000, 2, 'MOCK-REF', 0)`)
	mustExec(t, pool, `INSERT INTO wallet_ledger (id, seller_id, type, amount, status, reference_id) VALUES
		('c-partial', 's1', 'ORDER_SETTLEMENT', 500000, 'COMPLETED', 'tx-partial'),
		('d-partial', 's1', 'REFUND_DEDUCTION', -200000, 'COMPLETED', 'tx-partial'),
		('c-full',    's2', 'ORDER_SETTLEMENT', 300000, 'COMPLETED', 'tx-full'),
		('d-full',    's2', 'REFUND_DEDUCTION', -300000, 'COMPLETED', 'tx-full'),
		('c-paid',    's3', 'ORDER_SETTLEMENT', 100000, 'COMPLETED', 'tx-paid'),
		('c-pre6',    's4', 'ORDER_SETTLEMENT', 400000, 'COMPLETED', 'tx-pre6')`)

	for _, q := range precheck0007 {
		if n := countRows(t, pool, q); n != 0 {
			t.Fatalf("pre-check %q returned %d rows, want 0", q, n)
		}
	}
	balances := func() map[string]int64 {
		rows, err := pool.Query(context.Background(), `SELECT seller_id, SUM(amount)::BIGINT FROM wallet_ledger GROUP BY seller_id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string]int64{}
		for rows.Next() {
			var s string
			var v int64
			if err := rows.Scan(&s, &v); err != nil {
				t.Fatal(err)
			}
			out[s] = v
		}
		return out
	}
	before := balances()
	ledgerBefore := scalar[string](t, pool, `SELECT string_agg(id || ':' || amount || ':' || COALESCE(reference_id, ''), ',' ORDER BY id) FROM wallet_ledger WHERE seller_id IN ('s3', 's4')`)

	applyReset(t, pool, up)

	// One LEGACY refund each for the two positive legacy refunds, none for the others.
	type refund struct {
		id, payment, source, sourceID string
		requested, amount             int64
	}
	rows, err := pool.Query(context.Background(), `SELECT id, payment_id, source, source_id, requested_amount, amount FROM payment_refunds ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	var got []refund
	for rows.Next() {
		var r refund
		if err := rows.Scan(&r.id, &r.payment, &r.source, &r.sourceID, &r.requested, &r.amount); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	rows.Close()
	want := []refund{
		{"legacy:tx-full", "tx-full", "LEGACY", "tx-full", 300000, 300000},
		{"legacy:tx-partial", "tx-partial", "LEGACY", "tx-partial", 200000, 200000},
	}
	if len(got) != len(want) {
		t.Fatalf("refunds = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("refund %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if r := scalar[string](t, pool, `SELECT reason FROM payment_refunds WHERE id = 'legacy:tx-partial'`); r != "REFUND:damaged" {
		t.Fatalf("legacy reason = %q", r)
	}
	if ref := scalar[string](t, pool, `SELECT reference_id FROM wallet_ledger WHERE id = 'd-partial'`); ref != "legacy:tx-partial" {
		t.Fatalf("partial deduction reference = %q", ref)
	}
	if ref := scalar[string](t, pool, `SELECT reference_id FROM wallet_ledger WHERE id = 'd-full'`); ref != "legacy:tx-full" {
		t.Fatalf("full deduction reference = %q", ref)
	}
	// Status and refunded amount unchanged; the pre-0006 row is untouched.
	for id, st := range map[string]int32{"tx-partial": 4, "tx-full": 4, "tx-pre6": 4, "tx-paid": 2} {
		if got := scalar[int32](t, pool, `SELECT status FROM payment_transactions WHERE id = $1`, id); got != st {
			t.Fatalf("%s status = %d, want %d", id, got, st)
		}
	}
	if v := scalar[int64](t, pool, `SELECT refunded_amount FROM payment_transactions WHERE id = 'tx-partial'`); v != 200000 {
		t.Fatalf("partial refunded_amount = %d", v)
	}
	if v := scalar[int64](t, pool, `SELECT refunded_amount FROM payment_transactions WHERE id = 'tx-pre6'`); v != 0 {
		t.Fatalf("pre-0006 refunded_amount = %d", v)
	}
	after := balances()
	for s, v := range before {
		if after[s] != v {
			t.Fatalf("seller %s balance %d -> %d", s, v, after[s])
		}
	}
	if after["s1"] != 300000 {
		t.Fatalf("s1 balance = %d, want 300000", after["s1"])
	}
	if l := scalar[string](t, pool, `SELECT string_agg(id || ':' || amount || ':' || COALESCE(reference_id, ''), ',' ORDER BY id) FROM wallet_ledger WHERE seller_id IN ('s3', 's4')`); l != ledgerBefore {
		t.Fatalf("untouched ledger rows changed: %s -> %s", ledgerBefore, l)
	}

	// The new status rules.
	wantCode(t, pool, "23514", "new REFUNDED with a partial amount",
		`INSERT INTO payment_transactions (id, order_id, buyer_id, amount, status, refunded_amount) VALUES ('n1', 'o', 'b', 500, 4, 200)`)
	wantCode(t, pool, "23514", "PARTIALLY_REFUNDED with refunded 0",
		`INSERT INTO payment_transactions (id, order_id, buyer_id, amount, status, refunded_amount) VALUES ('n2', 'o', 'b', 500, 5, 0)`)
	wantCode(t, pool, "23514", "PARTIALLY_REFUNDED with refunded = amount",
		`INSERT INTO payment_transactions (id, order_id, buyer_id, amount, status, refunded_amount) VALUES ('n3', 'o', 'b', 500, 5, 500)`)
	wantCode(t, pool, "23514", "a PAID payment moved to REFUNDED partially",
		`UPDATE payment_transactions SET status = 4, refunded_amount = 50000 WHERE id = 'tx-paid'`)
	mustExec(t, pool, `INSERT INTO payment_transactions (id, order_id, buyer_id, amount, status, refunded_amount) VALUES ('n4', 'o', 'b', 500, 4, 500)`)
	wantCode(t, pool, "23514", "applied 0 outside RETURN",
		`INSERT INTO payment_refunds (id, payment_id, source, source_id, requested_amount, amount) VALUES ('rpc:z', 'tx-paid', 'SELLER_OR_ADMIN', 'z', 10, 0)`)
	wantCode(t, pool, "23514", "applied above requested",
		`INSERT INTO payment_refunds (id, payment_id, source, source_id, requested_amount, amount) VALUES ('return:z', 'tx-paid', 'RETURN', 'z', 10, 11)`)
	wantCode(t, pool, "23505", "duplicate refund key",
		`INSERT INTO payment_refunds (id, payment_id, source, source_id, requested_amount, amount) VALUES ('legacy:tx-full', 'tx-paid', 'LEGACY', 'x', 10, 10)`)

	// A new partial refund under the new model, so the down migration has a 5 to map.
	mustExec(t, pool, `UPDATE payment_transactions SET status = 5, refunded_amount = 40000 WHERE id = 'tx-paid'`)
	mustExec(t, pool, `INSERT INTO payment_refunds (id, payment_id, source, source_id, requested_amount, amount) VALUES ('rpc:r1', 'tx-paid', 'SELLER_OR_ADMIN', 'r1', 40000, 40000)`)
	mustExec(t, pool, `INSERT INTO wallet_ledger (id, seller_id, type, amount, status, reference_id) VALUES ('d-paid', 's3', 'REFUND_DEDUCTION', -40000, 'COMPLETED', 'rpc:r1')`)

	applyReset(t, pool, down)

	if ref := scalar[string](t, pool, `SELECT reference_id FROM wallet_ledger WHERE id = 'd-partial'`); ref != "tx-partial" {
		t.Fatalf("after down, partial deduction reference = %q", ref)
	}
	if ref := scalar[string](t, pool, `SELECT reference_id FROM wallet_ledger WHERE id = 'd-full'`); ref != "tx-full" {
		t.Fatalf("after down, full deduction reference = %q", ref)
	}
	if ref := scalar[string](t, pool, `SELECT reference_id FROM wallet_ledger WHERE id = 'd-paid'`); ref != "rpc:r1" {
		t.Fatalf("after down, new deduction reference = %q", ref)
	}
	if st := scalar[int32](t, pool, `SELECT status FROM payment_transactions WHERE id = 'tx-paid'`); st != 4 {
		t.Fatalf("after down, PARTIALLY_REFUNDED maps to %d, want 4", st)
	}
	if n := scalar[int64](t, pool, `SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'payment_refunds'`); n != 0 {
		t.Fatal("after down, payment_refunds still exists")
	}
	if l := scalar[int32](t, pool, `SELECT character_maximum_length FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'wallet_ledger' AND column_name = 'reference_id'`); l != 64 {
		t.Fatalf("after down, reference_id length = %d, want 64", l)
	}
	if b := balances()["s1"]; b != 300000 {
		t.Fatalf("after down, s1 balance = %d, want 300000", b)
	}
	// Up applies again over the rolled-back data.
	applyReset(t, pool, up)
	if ref := scalar[string](t, pool, `SELECT reference_id FROM wallet_ledger WHERE id = 'd-partial'`); ref != "legacy:tx-partial" {
		t.Fatalf("re-up, partial deduction reference = %q", ref)
	}
}

// The down migration keeps reference_id at 96 when a longer reference exists.
func TestMigration0007_DownKeepsWideReferences_Postgres(t *testing.T) {
	pool := pgtest.EmptyPool(t)
	up, down := migration0007Files(t, pool)
	pgtest.Apply(t, pool, up)
	long := "rpc:" + strings.Repeat("x", 64)
	mustExec(t, pool, `INSERT INTO wallet_ledger (id, seller_id, type, amount, status, reference_id) VALUES ('w', 's', 'REFUND_DEDUCTION', -1, 'COMPLETED', $1)`, long)
	applyReset(t, pool, down)
	if l := scalar[int32](t, pool, `SELECT character_maximum_length FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'wallet_ledger' AND column_name = 'reference_id'`); l != 96 {
		t.Fatalf("reference_id length = %d, want 96", l)
	}
}
