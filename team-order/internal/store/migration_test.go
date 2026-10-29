package store_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const lifecycleMigration = "0009_order_lifecycle_completion"

var (
	orderLifecycleColumns  = []string{"shipped_at", "auto_complete_at", "completed_at", "return_window_ends_at", "ship_by"}
	returnLifecycleColumns = []string{"auto_approve_at"}
)

// migrationConn gives the test a connection whose search_path is a private schema,
// so the migrations can be applied step by step on an empty database without
// disturbing the shared test schema (pgPool applies every migration to it).
type migrationConn struct {
	t    *testing.T
	ctx  context.Context
	conn *pgxpool.Conn
	sch  string
}

func newMigrationConn(t *testing.T) *migrationConn {
	t.Helper()
	pool := pgPool(t)
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sch := "mig_" + strings.ReplaceAll(uid("t"), "-", "")[:20]
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+sch); err != nil {
		conn.Release()
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, "SET search_path TO "+sch); err != nil {
		conn.Release()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(ctx, "RESET search_path")
		_, _ = conn.Exec(ctx, "DROP SCHEMA IF EXISTS "+sch+" CASCADE")
		conn.Release()
	})
	return &migrationConn{t: t, ctx: ctx, conn: conn, sch: sch}
}

func (m *migrationConn) exec(sql string, args ...any) {
	m.t.Helper()
	if _, err := m.conn.Exec(m.ctx, sql, args...); err != nil {
		m.t.Fatalf("exec %q: %v", sql, err)
	}
}

// apply runs the migration files of the given direction whose name sorts at or
// below (up) / is exactly (down) the selector; selector "" means all up files.
func (m *migrationConn) applyFile(name string) {
	m.t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
	if err != nil {
		m.t.Fatalf("read migration %s: %v", name, err)
	}
	if _, err := m.conn.Exec(m.ctx, string(b)); err != nil {
		m.t.Fatalf("apply migration %s: %v", name, err)
	}
}

// applyUpTo applies every up migration that precedes the lifecycle migration.
func (m *migrationConn) applyBefore() {
	m.t.Helper()
	entries, err := os.ReadDir(filepath.Join("..", "..", "migrations"))
	if err != nil {
		m.t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasSuffix(n, ".up.sql") && n < lifecycleMigration {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	if len(names) < 8 {
		m.t.Fatalf("expected migrations 0001..0008 before the lifecycle migration, found %v", names)
	}
	for _, n := range names {
		m.applyFile(n)
	}
}

func (m *migrationConn) columns(table string) map[string]bool {
	m.t.Helper()
	rows, err := m.conn.Query(m.ctx,
		`SELECT column_name FROM information_schema.columns WHERE table_schema = $1 AND table_name = $2`, m.sch, table)
	if err != nil {
		m.t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			m.t.Fatal(err)
		}
		out[c] = true
	}
	return out
}

// indexDefs returns the definitions of the indexes of table that are not primary keys.
func (m *migrationConn) indexDefs(table string) []string {
	m.t.Helper()
	rows, err := m.conn.Query(m.ctx,
		`SELECT indexdef FROM pg_indexes WHERE schemaname = $1 AND tablename = $2`, m.sch, table)
	if err != nil {
		m.t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			m.t.Fatal(err)
		}
		out = append(out, d)
	}
	return out
}

func (m *migrationConn) count(sql string) int {
	m.t.Helper()
	var n int
	if err := m.conn.QueryRow(m.ctx, sql).Scan(&n); err != nil {
		m.t.Fatal(err)
	}
	return n
}

func (m *migrationConn) insertOrder(id string, status, method int, paidAt *time.Time, created, updated time.Time) {
	m.t.Helper()
	m.exec(`INSERT INTO orders (id, buyer_id, seller_id, status, total_amount, payment_method, paid_at, created_at, updated_at)
	        VALUES ($1, 'buyer', 'seller', $2, 1000, $3, $4, $5, $6)`, id, status, method, paidAt, created, updated)
}

func (m *migrationConn) insertReturn(id, orderID string, status int) {
	m.t.Helper()
	m.exec(`INSERT INTO order_returns (id, order_id, buyer_id, seller_id, reason, refund_amount, status)
	        VALUES ($1, $2, 'buyer', 'seller', 'damaged', 100, $3)`, id, orderID, status)
}

func (m *migrationConn) ts(sql string, args ...any) *time.Time {
	m.t.Helper()
	var v *time.Time
	if err := m.conn.QueryRow(m.ctx, sql, args...).Scan(&v); err != nil {
		m.t.Fatal(err)
	}
	return v
}

func (m *migrationConn) clock() time.Time {
	m.t.Helper()
	var v time.Time
	if err := m.conn.QueryRow(m.ctx, `SELECT clock_timestamp()`).Scan(&v); err != nil {
		m.t.Fatal(err)
	}
	return v
}

func within(got *time.Time, lo, hi time.Time) bool {
	return got != nil && !got.Before(lo) && !got.After(hi)
}

func sameInstant(got *time.Time, want time.Time) bool {
	return got != nil && got.Sub(want).Abs() < time.Millisecond
}

const (
	stPending, stPaid, stShipped, stCompleted, stCancelled = 1, 2, 3, 4, 5
	mCOD, mMomo                                            = 1, 2
	rPending, rApproved                                    = 1, 2
	day                                                    = 24 * time.Hour
)

// seedLifecycleRows inserts one order of every backfill class plus the non-targets.
func seedLifecycleRows(m *migrationConn, old time.Time) {
	paid := old.Add(-6 * time.Hour)
	m.insertOrder("o-shipped", stShipped, mMomo, &paid, old.Add(-10*day), old)
	m.insertOrder("o-completed", stCompleted, mMomo, &paid, old.Add(-30*day), old.Add(-3*day))
	m.insertOrder("o-paid", stPaid, mMomo, &paid, old.Add(-2*day), old)
	m.insertOrder("o-paid-unstamped", stPaid, mMomo, nil, old.Add(-5*day), old)
	m.insertOrder("o-cod-pending", stPending, mCOD, nil, old.Add(-1*day), old)
	m.insertOrder("o-online-pending", stPending, mMomo, nil, old.Add(-1*day), old)
	m.insertOrder("o-cancelled", stCancelled, mMomo, nil, old.Add(-9*day), old)
	m.insertReturn("r-pending", "o-shipped", rPending)
	m.insertReturn("r-approved", "o-completed", rApproved)
}

// Order receipt: Migration backfills deadlines without completing anything
func TestMigrationBackfillsLifecycleDeadlines(t *testing.T) {
	m := newMigrationConn(t)
	m.applyBefore()
	old := time.Now().UTC().Truncate(time.Second).Add(-12 * time.Hour)
	seedLifecycleRows(m, old)
	outboxBefore := m.count(`SELECT count(*) FROM order_outbox_events`)

	t0 := m.clock()
	m.applyFile(lifecycleMigration + ".up.sql")
	t1 := m.clock()

	orderTS := func(id, col string) *time.Time {
		return m.ts(`SELECT `+col+` FROM orders WHERE id = $1`, id)
	}
	returnTS := func(id, col string) *time.Time {
		return m.ts(`SELECT `+col+` FROM order_returns WHERE id = $1`, id)
	}

	t.Run("shipped order gets shipped_at and auto_complete_at of migration time plus 7 days", func(t *testing.T) {
		if orderTS("o-shipped", "shipped_at") == nil {
			t.Fatal("shipped_at must be set for an existing SHIPPED order")
		}
		if got := orderTS("o-shipped", "auto_complete_at"); !within(got, t0.Add(7*day), t1.Add(7*day)) {
			t.Fatalf("auto_complete_at = %v, want migration time + 7d in [%v, %v]", got, t0.Add(7*day), t1.Add(7*day))
		}
		if orderTS("o-shipped", "completed_at") != nil || orderTS("o-shipped", "return_window_ends_at") != nil {
			t.Fatal("a SHIPPED order must not be given completion stamps")
		}
	})

	t.Run("completed order gets completed_at = updated_at and window 15 days later", func(t *testing.T) {
		updated := old.Add(-3 * day)
		if got := orderTS("o-completed", "completed_at"); !sameInstant(got, updated) {
			t.Fatalf("completed_at = %v, want updated_at %v", got, updated)
		}
		if got := orderTS("o-completed", "return_window_ends_at"); !sameInstant(got, updated.Add(15*day)) {
			t.Fatalf("return_window_ends_at = %v, want %v", got, updated.Add(15*day))
		}
		if orderTS("o-completed", "auto_complete_at") != nil {
			t.Fatal("a COMPLETED order must have no auto_complete_at")
		}
	})

	t.Run("paid and cod pending orders get ship_by from paid_at or created_at plus 2 days", func(t *testing.T) {
		paid := old.Add(-6 * time.Hour)
		if got := orderTS("o-paid", "ship_by"); !sameInstant(got, paid.Add(2*day)) {
			t.Fatalf("paid order ship_by = %v, want paid_at+2d %v", got, paid.Add(2*day))
		}
		if got := orderTS("o-paid-unstamped", "ship_by"); !sameInstant(got, old.Add(-5*day).Add(2*day)) {
			t.Fatalf("paid order without paid_at ship_by = %v, want created_at+2d", got)
		}
		if got := orderTS("o-cod-pending", "ship_by"); !sameInstant(got, old.Add(-1*day).Add(2*day)) {
			t.Fatalf("cod pending ship_by = %v, want created_at+2d", got)
		}
	})

	t.Run("online pending and cancelled orders get no deadline", func(t *testing.T) {
		for _, id := range []string{"o-online-pending", "o-cancelled"} {
			for _, col := range orderLifecycleColumns {
				if got := orderTS(id, col); got != nil {
					t.Fatalf("%s.%s = %v, want NULL", id, col, got)
				}
			}
		}
		if orderTS("o-paid", "auto_complete_at") != nil || orderTS("o-paid", "completed_at") != nil {
			t.Fatal("a PAID order must get no shipped or completion stamps")
		}
	})

	t.Run("pending return gets auto_approve_at of migration time plus 3 days, others none", func(t *testing.T) {
		if got := returnTS("r-pending", "auto_approve_at"); !within(got, t0.Add(3*day), t1.Add(3*day)) {
			t.Fatalf("auto_approve_at = %v, want migration time + 3d in [%v, %v]", got, t0.Add(3*day), t1.Add(3*day))
		}
		if got := returnTS("r-approved", "auto_approve_at"); got != nil {
			t.Fatalf("an APPROVED return must get no auto_approve_at, got %v", got)
		}
	})

	t.Run("no status changes and no outbox rows", func(t *testing.T) {
		want := map[string]int{
			"o-shipped": stShipped, "o-completed": stCompleted, "o-paid": stPaid,
			"o-paid-unstamped": stPaid, "o-cod-pending": stPending, "o-online-pending": stPending, "o-cancelled": stCancelled,
		}
		for id, st := range want {
			if n := m.count(`SELECT count(*) FROM orders WHERE id = '` + id + `' AND status = ` + itoa(st)); n != 1 {
				t.Fatalf("order %s status changed by the migration", id)
			}
		}
		if m.count(`SELECT count(*) FROM order_returns WHERE id = 'r-pending' AND status = 1`) != 1 ||
			m.count(`SELECT count(*) FROM order_returns WHERE id = 'r-approved' AND status = 2`) != 1 {
			t.Fatal("a return status was changed by the migration")
		}
		if got := m.count(`SELECT count(*) FROM order_outbox_events`); got != outboxBefore {
			t.Fatalf("outbox rows = %d, want %d (migration must emit nothing)", got, outboxBefore)
		}
	})

	t.Run("sweep indexes exist on the new deadline columns", func(t *testing.T) {
		has := func(table, col string) bool {
			for _, d := range m.indexDefs(table) {
				if strings.Contains(d, col) {
					return true
				}
			}
			return false
		}
		if !has("orders", "auto_complete_at") {
			t.Fatal("no index on orders.auto_complete_at for the sweep")
		}
		if !has("order_returns", "auto_approve_at") {
			t.Fatal("no index on order_returns.auto_approve_at for the sweep")
		}
	})
}

// Order receipt: Migration backfills deadlines without completing anything
// (the harness re-applies every *.up.sql, so a second run must not move a stamped deadline)
func TestMigrationUpIsRerunnableAndKeepsDeadlines(t *testing.T) {
	m := newMigrationConn(t)
	m.applyBefore()
	seedLifecycleRows(m, time.Now().UTC().Truncate(time.Second).Add(-12*time.Hour))
	m.applyFile(lifecycleMigration + ".up.sql")
	first := m.ts(`SELECT auto_complete_at FROM orders WHERE id = 'o-shipped'`)
	firstRet := m.ts(`SELECT auto_approve_at FROM order_returns WHERE id = 'r-pending'`)
	if first == nil || firstRet == nil {
		t.Fatal("first run must stamp the deadlines")
	}
	m.applyFile(lifecycleMigration + ".up.sql")
	if got := m.ts(`SELECT auto_complete_at FROM orders WHERE id = 'o-shipped'`); !sameInstant(got, *first) {
		t.Fatalf("auto_complete_at moved on re-run: %v -> %v", first, got)
	}
	if got := m.ts(`SELECT auto_approve_at FROM order_returns WHERE id = 'r-pending'`); !sameInstant(got, *firstRet) {
		t.Fatalf("auto_approve_at moved on re-run: %v -> %v", firstRet, got)
	}
}

// Order receipt: The down migration restores the old schema
func TestMigrationDownDropsLifecycleColumns(t *testing.T) {
	m := newMigrationConn(t)
	m.applyBefore()
	baseOrders, baseReturns := m.columns("orders"), m.columns("order_returns")
	baseOrderIdx, baseReturnIdx := len(m.indexDefs("orders")), len(m.indexDefs("order_returns"))
	seedLifecycleRows(m, time.Now().UTC().Truncate(time.Second).Add(-12*time.Hour))

	m.applyFile(lifecycleMigration + ".up.sql")
	for _, c := range orderLifecycleColumns {
		if !m.columns("orders")[c] {
			t.Fatalf("up migration did not add orders.%s", c)
		}
	}
	for _, c := range returnLifecycleColumns {
		if !m.columns("order_returns")[c] {
			t.Fatalf("up migration did not add order_returns.%s", c)
		}
	}

	m.applyFile(lifecycleMigration + ".down.sql")

	t.Run("new columns are gone and prior columns remain", func(t *testing.T) {
		gotO, gotR := m.columns("orders"), m.columns("order_returns")
		for _, c := range orderLifecycleColumns {
			if gotO[c] {
				t.Fatalf("orders.%s still exists after down", c)
			}
		}
		for _, c := range returnLifecycleColumns {
			if gotR[c] {
				t.Fatalf("order_returns.%s still exists after down", c)
			}
		}
		for c := range baseOrders {
			if !gotO[c] {
				t.Fatalf("down dropped pre-existing column orders.%s", c)
			}
		}
		for c := range baseReturns {
			if !gotR[c] {
				t.Fatalf("down dropped pre-existing column order_returns.%s", c)
			}
		}
	})

	t.Run("the sweep indexes are gone", func(t *testing.T) {
		if n := len(m.indexDefs("orders")); n != baseOrderIdx {
			t.Fatalf("orders has %d indexes after down, want %d", n, baseOrderIdx)
		}
		if n := len(m.indexDefs("order_returns")); n != baseReturnIdx {
			t.Fatalf("order_returns has %d indexes after down, want %d", n, baseReturnIdx)
		}
	})

	t.Run("previous order and return queries still work and data is kept", func(t *testing.T) {
		if n := m.count(`SELECT count(*) FROM orders WHERE status = 3 AND payment_due_at IS NULL AND cancel_reason = 0`); n != 1 {
			t.Fatalf("previous order query returned %d rows, want 1", n)
		}
		if n := m.count(`SELECT count(*) FROM order_returns WHERE status = 1`); n != 1 {
			t.Fatalf("previous return query returned %d rows, want 1", n)
		}
	})

	t.Run("down is re-runnable", func(_ *testing.T) {
		m.applyFile(lifecycleMigration + ".down.sql")
	})
}

func itoa(n int) string {
	return string(rune('0' + n))
}
