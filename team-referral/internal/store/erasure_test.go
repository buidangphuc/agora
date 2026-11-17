package store_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-referral/internal/store"
)

func execAll(t *testing.T, pool *pgxpool.Pool, stmts ...string) {
	t.Helper()
	for _, s := range stmts {
		if _, err := pool.Exec(context.Background(), s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func snapshot(t *testing.T, pool *pgxpool.Pool, query string) string {
	t.Helper()
	var s string
	q := `SELECT COALESCE(string_agg(x, '|' ORDER BY x), '') FROM (` + query + `) q(x)`
	if err := pool.QueryRow(context.Background(), q).Scan(&s); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return s
}

const (
	identityRows = `SELECT row_to_json(c)::text FROM referral_codes c WHERE user_id = '%U%'
		UNION ALL SELECT row_to_json(a)::text FROM referral_accounts a WHERE user_id = '%U%'
		UNION ALL SELECT row_to_json(b)::text FROM referral_buyer_orders b WHERE buyer_id = '%U%'`
	keptRows = `SELECT row_to_json(r)::text FROM referrals r
		UNION ALL SELECT row_to_json(w)::text FROM referral_rewards w`
	allRows = `SELECT row_to_json(c)::text FROM referral_codes c
		UNION ALL SELECT row_to_json(a)::text FROM referral_accounts a
		UNION ALL SELECT row_to_json(b)::text FROM referral_buyer_orders b
		UNION ALL ` + keptRows
)

func rowsOf(t *testing.T, pool *pgxpool.Pool, user string) string {
	return snapshot(t, pool, strings.ReplaceAll(identityRows, "%U%", user))
}

func seedReferralUser(t *testing.T, pool *pgxpool.Pool, user string) {
	execAll(t, pool,
		`INSERT INTO referral_codes (user_id, code) VALUES ('`+user+`', 'ABC-`+user+`')`,
		`INSERT INTO referral_accounts (user_id, registered_at) VALUES ('`+user+`', now())`,
		`INSERT INTO referral_buyer_orders (buyer_id, first_completed_at) VALUES ('`+user+`', now())`)
}

func seedErasureWorld(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := schemaPool(t)
	seedReferralUser(t, pool, "U")
	seedReferralUser(t, pool, "W")
	execAll(t, pool,
		`INSERT INTO referrals (referrer_id, referee_id, code, status) VALUES ('U','r1','ABC-U','REWARDED'), ('U','r2','ABC-U','ATTRIBUTED')`,
		`INSERT INTO referral_rewards (id, user_id, amount, reason, status, release_at) VALUES
			('rw1','U',50,'invitee_first_order','RELEASED', now()), ('rw2','U',50,'invitee_first_order','PENDING', now()), ('rw3','U',50,'invitee_first_order','VOID', now())`)
	return pool
}

func TestEraseUser(t *testing.T) {
	// REF-1, REF-3, REF-5
	pool := seedErasureWorld(t)
	r := store.NewPostgresReferralRepo(pool)
	wBefore := rowsOf(t, pool, "W")
	keptBefore := snapshot(t, pool, keptRows)
	if rowsOf(t, pool, "U") == "" || wBefore == "" {
		t.Fatal("seed missing")
	}

	if err := r.EraseUser(context.Background(), "U"); err != nil {
		t.Fatalf("EraseUser: %v", err)
	}
	if got := rowsOf(t, pool, "U"); got != "" {
		t.Errorf("REF-1 rows of U remain: %s", got)
	}
	if got := snapshot(t, pool, keptRows); got != keptBefore {
		t.Errorf("REF-1 referrals or rewards changed:\nbefore %s\nafter  %s", keptBefore, got)
	}
	if n := count(t, pool, `SELECT count(*) FROM referral_rewards WHERE user_id = 'U'`); n != 3 {
		t.Errorf("REF-1 reward rows = %d, want 3", n)
	}
	if got := rowsOf(t, pool, "W"); got != wBefore {
		t.Errorf("REF-3 W's rows changed:\nbefore %s\nafter  %s", wBefore, got)
	}

	all := snapshot(t, pool, allRows)
	if err := r.EraseUser(context.Background(), "nobody"); err != nil {
		t.Fatalf("REF-5 EraseUser for a user without referral data: %v", err)
	}
	if got := snapshot(t, pool, allRows); got != all {
		t.Errorf("REF-5 no-data user changed rows:\nbefore %s\nafter  %s", all, got)
	}
}

func TestEraseUserIdempotent(t *testing.T) {
	// REF-4
	pool := seedErasureWorld(t)
	r := store.NewPostgresReferralRepo(pool)
	if err := r.EraseUser(context.Background(), "U"); err != nil {
		t.Fatal(err)
	}
	after := snapshot(t, pool, allRows)
	for i := 0; i < 2; i++ {
		if err := r.EraseUser(context.Background(), "U"); err != nil {
			t.Fatalf("redelivery %d: %v", i, err)
		}
	}
	if got := snapshot(t, pool, allRows); got != after {
		t.Errorf("REF-4 redelivery changed rows:\nbefore %s\nafter  %s", after, got)
	}
}

func TestEraseUserCodeNoLongerResolves(t *testing.T) {
	// REF-2
	pool := seedErasureWorld(t)
	r := store.NewPostgresReferralRepo(pool)
	if err := r.EraseUser(context.Background(), "U"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.OwnerOfCode(context.Background(), "ABC-U"); err == nil {
		t.Fatal("REF-2 OwnerOfCode still resolves the erased user's code")
	}
	before := count(t, pool, `SELECT count(*) FROM referrals`)
	signup(t, r, "newbie", "ABC-U", t0)
	if n := count(t, pool, `SELECT count(*) FROM referrals`); n != before {
		t.Errorf("REF-2 referrals = %d after signup with an erased code, want %d", n, before)
	}
	if n := count(t, pool, `SELECT count(*) FROM referrals WHERE referee_id = 'newbie'`); n != 0 {
		t.Error("REF-2 the signup was attributed to the erased user")
	}
}

func TestEraseUserRollsBack(t *testing.T) {
	// REF-7
	pool := seedErasureWorld(t)
	r := store.NewPostgresReferralRepo(pool)
	execAll(t, pool, `CREATE FUNCTION erasure_fail() RETURNS trigger AS $$
BEGIN RAISE EXCEPTION 'injected failure'; END $$ LANGUAGE plpgsql`,
		`CREATE TRIGGER erasure_fail BEFORE DELETE ON referral_accounts FOR EACH ROW EXECUTE FUNCTION erasure_fail()`)
	before := snapshot(t, pool, allRows)

	err := r.EraseUser(context.Background(), "U")
	if err == nil || !strings.Contains(err.Error(), "injected failure") {
		t.Fatalf("EraseUser = %v, want the store error returned for retry", err)
	}
	if got := snapshot(t, pool, allRows); got != before {
		t.Fatalf("REF-7 partial erasure after failure:\nbefore %s\nafter  %s", before, got)
	}

	execAll(t, pool, `DROP TRIGGER erasure_fail ON referral_accounts`)
	if err := r.EraseUser(context.Background(), "U"); err != nil {
		t.Fatalf("retry after recovery: %v", err)
	}
	if got := rowsOf(t, pool, "U"); got != "" {
		t.Errorf("REF-7 rows remain after recovery: %s", got)
	}
}
