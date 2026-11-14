package consumer_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/buidangphuc/team-promotion/internal/consumer"
)

func seedImpression(t *testing.T, e *env, token string, age time.Duration) {
	t.Helper()
	created := t0.Add(-age)
	if _, err := e.pool.Exec(context.Background(), `INSERT INTO ad_impressions (token, campaign_id, listing_id, created_at, expires_at)
		VALUES ($1, 'camp-1', 'listing-1', $2, $3)`, token, created, created.Add(30*time.Minute)); err != nil {
		t.Fatalf("seed impression %s: %v", token, err)
	}
}

func seedCampaign(t *testing.T, e *env) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), `INSERT INTO ad_campaigns (id, seller_id, listing_id, budget, bid, status, created_at, spent, impressions, clicks)
		VALUES ('camp-1', 'seller-1', 'listing-1', 5000, 800, 1, $1, 1600, 12, 2)`, t0.Add(-60*24*time.Hour)); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
}

func TestSweeperPurgesOldImpressions(t *testing.T) {
	e := newEnv(t)
	seedCampaign(t, e)
	seedImpression(t, e, "old", 31*24*time.Hour)
	seedImpression(t, e, "recent", 29*24*time.Hour)

	if _, err := e.sweeper.Sweep(context.Background()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if n := count(t, e.pool, `SELECT count(*) FROM ad_impressions WHERE token = 'old'`); n != 0 {
		t.Fatalf("31-day-old row still present")
	}
	if n := count(t, e.pool, `SELECT count(*) FROM ad_impressions WHERE token = 'recent'`); n != 1 {
		t.Fatalf("29-day-old row was deleted")
	}
	var spent, imps, clicks int64
	if err := e.pool.QueryRow(context.Background(), `SELECT spent, impressions, clicks FROM ad_campaigns WHERE id = 'camp-1'`).Scan(&spent, &imps, &clicks); err != nil {
		t.Fatal(err)
	}
	if spent != 1600 || imps != 12 || clicks != 2 {
		t.Fatalf("campaign counters changed: spent=%d impressions=%d clicks=%d", spent, imps, clicks)
	}
}

func TestSweeperPurgesInBatchesOfAtMost100(t *testing.T) {
	e := newEnv(t)
	seedCampaign(t, e)
	if _, err := e.pool.Exec(context.Background(), `INSERT INTO ad_impressions (token, campaign_id, listing_id, created_at, expires_at)
		SELECT 'bulk-'||g, 'camp-1', 'listing-1', $1::timestamptz, $1::timestamptz + interval '30 minutes' FROM generate_series(1, 250) g`,
		t0.Add(-40*24*time.Hour)); err != nil {
		t.Fatalf("seed bulk: %v", err)
	}
	if _, err := e.pool.Exec(context.Background(), `CREATE TABLE purge_stmt_log (n BIGINT);
		CREATE FUNCTION log_purge_stmt() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN INSERT INTO purge_stmt_log SELECT count(*) FROM old_rows; RETURN NULL; END $$;
		CREATE TRIGGER log_purge AFTER DELETE ON ad_impressions REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION log_purge_stmt()`); err != nil {
		t.Fatalf("install statement logger: %v", err)
	}

	if _, err := e.sweeper.Sweep(context.Background()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if n := count(t, e.pool, `SELECT count(*) FROM ad_impressions`); n != 0 {
		t.Fatalf("rows left = %d, want 0", n)
	}
	if n := count(t, e.pool, `SELECT count(*) FROM purge_stmt_log WHERE n > 100`); n != 0 {
		t.Fatalf("a delete statement removed more than 100 rows")
	}
	if n := count(t, e.pool, `SELECT count(*) FROM purge_stmt_log WHERE n > 0`); n < 3 {
		t.Fatalf("delete statements = %d, want at least 3 for 250 rows", n)
	}
}

func TestSweeperPurgeFailureDoesNotBlockVouchers(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	pool := newPool(t)
	clock := t0
	cfg := testConfig(func() time.Time { return clock })
	cfg.Logger = logger
	e := &env{
		pool: pool, cfg: cfg, clock: &clock,
		identity: consumer.NewIdentityHandler(pool, cfg),
		referral: consumer.NewReferralHandler(pool, cfg),
		sweeper:  consumer.NewSweeper(pool, cfg),
	}
	seedCampaign(t, e)
	seedImpression(t, e, "old", 31*24*time.Hour)
	e.mustRegister(t, "user-timeout", "C")
	if _, err := pool.Exec(context.Background(), `CREATE FUNCTION fail_purge() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'purge boom'; END $$;
		CREATE TRIGGER fail_purge BEFORE DELETE ON ad_impressions FOR EACH ROW EXECUTE FUNCTION fail_purge()`); err != nil {
		t.Fatalf("install failing trigger: %v", err)
	}
	clock = t0.Add(verdictTimeout + time.Minute)
	seedImpression(t, e, "old2", 31*24*time.Hour+verdictTimeout+time.Minute)

	n, err := e.sweeper.Sweep(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("Sweep = %d, %v, want 1 voucher issued and no error", n, err)
	}
	e.requireOne(t, "user-timeout", 20000)
	if c := count(t, pool, `SELECT count(*) FROM ad_impressions WHERE token = 'old'`); c != 1 {
		t.Fatalf("old row count = %d, want 1 (delete failed)", c)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"level":"ERROR"`)) {
		t.Fatalf("purge failure was not logged at error level:\n%s", buf.String())
	}
}
