package store_test

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-promotion/internal/store"
)

var erasedAt = time.Date(2026, 11, 14, 3, 0, 0, 0, time.UTC)

func seedErasureVoucher(t *testing.T, pool *pgxpool.Pool, id, code string, kind int32, owner, seller string) {
	t.Helper()
	scope := 2
	if seller != "" {
		scope = 1
	}
	_, err := pool.Exec(t.Context(), `INSERT INTO vouchers (id, code, scope, seller_id, discount_type, discount_value, quota, kind, owner_user_id, active)
		VALUES ($1,$2,$3,$4,2,20000,1,$5,$6,true)`, id, code, scope, seller, kind, owner)
	if err != nil {
		t.Fatalf("seed voucher %s: %v", id, err)
	}
}

func seedErasureState(t *testing.T, pool *pgxpool.Pool, user string) {
	t.Helper()
	_, err := pool.Exec(t.Context(), `INSERT INTO new_user_voucher_state (user_id, registered, has_code, verdict, registered_at, issued_at, updated_at)
		VALUES ($1,true,false,'',now(),now(),now())`, user)
	if err != nil {
		t.Fatalf("seed state %s: %v", user, err)
	}
}

func seedErasureReservation(t *testing.T, pool *pgxpool.Pool, id, voucher, buyer string, amount int64) {
	t.Helper()
	_, err := pool.Exec(t.Context(), `INSERT INTO voucher_reservations (id, reservation_id, voucher_id, buyer_id, discount_amount, status, expires_at)
		VALUES ($1,$1,$2,$3,$4,'committed', now() + interval '1 hour')`, id, voucher, buyer, amount)
	if err != nil {
		t.Fatalf("seed reservation %s: %v", id, err)
	}
}

func rowJSON(t *testing.T, pool *pgxpool.Pool, query string, args ...any) string {
	t.Helper()
	var s string
	q := `SELECT COALESCE(string_agg(x, '|' ORDER BY x), '') FROM (` + query + `) q(x)`
	if err := pool.QueryRow(t.Context(), q, args...).Scan(&s); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return s
}

func voucherRows(t *testing.T, pool *pgxpool.Pool, ids ...string) string {
	return rowJSON(t, pool, `SELECT row_to_json(v)::text FROM vouchers v WHERE id = ANY($1)`, ids)
}

func stateRows(t *testing.T, pool *pgxpool.Pool, user string) string {
	return rowJSON(t, pool, `SELECT row_to_json(s)::text FROM new_user_voucher_state s WHERE user_id = $1`, user)
}

func reservationRows(t *testing.T, pool *pgxpool.Pool) string {
	return rowJSON(t, pool, `SELECT row_to_json(r)::text FROM voucher_reservations r`)
}

func everything(t *testing.T, pool *pgxpool.Pool) string {
	return rowJSON(t, pool, `SELECT row_to_json(v)::text FROM vouchers v
		UNION ALL SELECT row_to_json(s)::text FROM new_user_voucher_state s
		UNION ALL SELECT row_to_json(r)::text FROM voucher_reservations r`)
}

type seeded struct {
	pool *pgxpool.Pool
	repo *store.PostgresErasureRepository
}

func seedErasure(t *testing.T) seeded {
	t.Helper()
	pool := newTestPool(t)
	seedErasureVoucher(t, pool, "v-u", "NU-U", store.VoucherKindNewUser, "U", "")
	seedErasureState(t, pool, "U")
	seedErasureReservation(t, pool, "r-1", "v-u", "U", 20000)
	seedErasureReservation(t, pool, "r-2", "v-u", "U", 30000)
	seedErasureVoucher(t, pool, "v-w", "NU-W", store.VoucherKindNewUser, "W", "")
	seedErasureState(t, pool, "W")
	seedErasureVoucher(t, pool, "v-shop", "SHOP-U", store.VoucherKindStandard, "", "U")
	return seeded{pool: pool, repo: store.NewPostgresErasureRepository(pool)}
}

func (s seeded) deactivated(t *testing.T, id string) (bool, *time.Time) {
	t.Helper()
	var active bool
	var at *time.Time
	if err := s.pool.QueryRow(t.Context(), `SELECT active, deactivated_at FROM vouchers WHERE id = $1`, id).Scan(&active, &at); err != nil {
		t.Fatal(err)
	}
	return active, at
}

func TestEraseUser(t *testing.T) {
	// PRO-1, PRO-2, PRO-4
	s := seedErasure(t)
	reservationsBefore := reservationRows(t, s.pool)
	othersBefore := voucherRows(t, s.pool, "v-w", "v-shop") + stateRows(t, s.pool, "W")

	if err := s.repo.EraseUser(t.Context(), "U", erasedAt); err != nil {
		t.Fatalf("EraseUser: %v", err)
	}

	active, at := s.deactivated(t, "v-u")
	if active || at == nil || !at.Equal(erasedAt) {
		t.Errorf("PRO-1 owned voucher active=%v deactivated_at=%v, want false and %v", active, at, erasedAt)
	}
	if got := stateRows(t, s.pool, "U"); got != "" {
		t.Errorf("PRO-1 state row of U remains: %s", got)
	}
	if got := reservationRows(t, s.pool); got != reservationsBefore {
		t.Errorf("PRO-1 reservations changed:\nbefore %s\nafter  %s", reservationsBefore, got)
	}
	if n := erasureCount(t, s.pool, `SELECT count(*) FROM voucher_reservations WHERE buyer_id = 'U' AND status = 'committed' AND discount_amount IN (20000, 30000)`); n != 2 {
		t.Errorf("PRO-1 committed reservations with buyer and amount = %d, want 2", n)
	}
	if got := voucherRows(t, s.pool, "v-w", "v-shop") + stateRows(t, s.pool, "W"); got != othersBefore {
		t.Errorf("PRO-2 other vouchers or state changed:\nbefore %s\nafter  %s", othersBefore, got)
	}

	snapshot := everything(t, s.pool)
	if err := s.repo.EraseUser(t.Context(), "nobody", erasedAt.Add(time.Hour)); err != nil {
		t.Fatalf("PRO-4 EraseUser for an unknown user: %v", err)
	}
	if got := everything(t, s.pool); got != snapshot {
		t.Errorf("PRO-4 a no-data user changed rows:\nbefore %s\nafter  %s", snapshot, got)
	}
}

func TestEraseUserIdempotent(t *testing.T) {
	// PRO-3
	s := seedErasure(t)
	if err := s.repo.EraseUser(t.Context(), "U", erasedAt); err != nil {
		t.Fatalf("first: %v", err)
	}
	after := everything(t, s.pool)
	for i, at := range []time.Time{erasedAt, erasedAt.Add(2 * time.Hour)} {
		if err := s.repo.EraseUser(t.Context(), "U", at); err != nil {
			t.Fatalf("redelivery %d: %v", i, err)
		}
	}
	if got := everything(t, s.pool); got != after {
		t.Errorf("PRO-3 redelivery changed rows:\nbefore %s\nafter  %s", after, got)
	}
	if _, at := s.deactivated(t, "v-u"); at == nil || !at.Equal(erasedAt) {
		t.Errorf("PRO-3 deactivated_at = %v, want the first value %v", at, erasedAt)
	}
}

func TestEraseUserRollsBack(t *testing.T) {
	// PRO-6
	s := seedErasure(t)
	_, err := s.pool.Exec(t.Context(), `CREATE FUNCTION erasure_fail() RETURNS trigger AS $$
BEGIN RAISE EXCEPTION 'injected failure'; END $$ LANGUAGE plpgsql;
CREATE TRIGGER erasure_fail BEFORE DELETE ON new_user_voucher_state FOR EACH ROW EXECUTE FUNCTION erasure_fail();`)
	if err != nil {
		t.Fatalf("install failure: %v", err)
	}
	before := everything(t, s.pool)

	err = s.repo.EraseUser(t.Context(), "U", erasedAt)
	if err == nil || !strings.Contains(err.Error(), "injected failure") {
		t.Fatalf("EraseUser = %v, want the store error returned for retry", err)
	}
	if got := everything(t, s.pool); got != before {
		t.Fatalf("PRO-6 partial erasure after failure:\nbefore %s\nafter  %s", before, got)
	}
	if active, _ := s.deactivated(t, "v-u"); !active {
		t.Error("PRO-6 voucher deactivated although the transaction failed")
	}

	if _, err := s.pool.Exec(t.Context(), `DROP TRIGGER erasure_fail ON new_user_voucher_state`); err != nil {
		t.Fatal(err)
	}
	if err := s.repo.EraseUser(t.Context(), "U", erasedAt); err != nil {
		t.Fatalf("retry after recovery: %v", err)
	}
	if active, at := s.deactivated(t, "v-u"); active || at == nil {
		t.Errorf("PRO-6 after recovery active=%v deactivated_at=%v", active, at)
	}
	if got := stateRows(t, s.pool, "U"); got != "" {
		t.Errorf("PRO-6 state row remains after recovery: %s", got)
	}
}

func erasureCount(t *testing.T, pool *pgxpool.Pool, query string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), query).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}
