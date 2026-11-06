package store_test

import (
	"context"
	"fmt"
	"io/fs"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"

	"github.com/buidangphuc/team-referral/internal/store"
	"github.com/buidangphuc/team-referral/migrations"
)

const (
	typeAttributed = "platform.referral.v1.ReferralAttributed"
	typeReleased   = "platform.referral.v1.ReferralRewardReleased"
)

func count(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

func installOutboxFailure(t *testing.T, pool *pgxpool.Pool, when string) {
	t.Helper()
	sql := `CREATE OR REPLACE FUNCTION referral_outbox_fail() RETURNS trigger AS $$
BEGIN RAISE EXCEPTION 'outbox unavailable'; END $$ LANGUAGE plpgsql;
CREATE TRIGGER referral_outbox_fail BEFORE INSERT ON referral_outbox_events
FOR EACH ROW ` + when + ` EXECUTE FUNCTION referral_outbox_fail();`
	if _, err := pool.Exec(context.Background(), sql); err != nil {
		t.Fatalf("install failing trigger: %v", err)
	}
}

func removeOutboxFailure(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `DROP TRIGGER referral_outbox_fail ON referral_outbox_events`); err != nil {
		t.Fatalf("drop failing trigger: %v", err)
	}
}

func TestAttributionRollsBackWithOutbox(t *testing.T) {
	ctx := context.Background()
	pool := schemaPool(t)
	r := store.NewPostgresReferralRepo(pool)
	if _, err := r.CreateCode(ctx, "alice", "REFAAAA1111"); err != nil {
		t.Fatal(err)
	}
	installOutboxFailure(t, pool, "")

	in := store.UserRegisteredInput{EventID: "ev-1", UserID: "bob", ReferralCode: "REFAAAA1111", RegisteredAt: t0, At: t0, LifetimeCap: 50}
	if err := r.ApplyUserRegistered(ctx, in); err == nil {
		t.Fatal("ApplyUserRegistered succeeded although the outbox insert failed")
	}
	if n := count(t, pool, `SELECT count(*) FROM referrals`); n != 0 {
		t.Fatalf("referrals rows = %d after rollback, want 0", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM referral_processed_events`); n != 0 {
		t.Fatalf("processed events = %d after rollback, want 0 so the event is retried", n)
	}

	removeOutboxFailure(t, pool)
	if err := r.ApplyUserRegistered(ctx, in); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if n := count(t, pool, `SELECT count(*) FROM referrals WHERE referee_id = 'bob'`); n != 1 {
		t.Fatalf("referrals rows after retry = %d, want 1", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM referral_outbox_events WHERE event_type = $1`, typeAttributed); n != 1 {
		t.Fatalf("outbox rows after retry = %d, want 1", n)
	}
}

func TestLifetimeCapHoldsUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	pool := schemaPool(t)
	r := store.NewPostgresReferralRepo(pool)
	if _, err := r.CreateCode(ctx, "alice", "REFAAAA1111"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 49; i++ {
		u := fmt.Sprintf("seed%02d", i)
		signup(t, r, u, "REFAAAA1111", t0)
		complete(t, r, u, "so-"+u, t0.Add(time.Hour), t0, 50)
	}
	signup(t, r, "bob", "REFAAAA1111", t0)
	signup(t, r, "carol", "REFAAAA1111", t0)

	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, buyer := range []string{"bob", "carol"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_ = r.ApplyOrderCompleted(ctx, store.OrderCompletedInput{
				EventID: "oc-" + buyer, OrderID: "o-" + buyer, BuyerID: buyer, CompletedAt: t0,
				ReleaseAt: t0.Add(time.Hour), At: t0, RewardXU: 10000, LifetimeCap: 50,
			})
		}()
	}
	close(start)
	wg.Wait()

	if n := count(t, pool, `SELECT count(*) FROM referral_rewards WHERE status <> 'VOID'`); n != 50 {
		t.Fatalf("non-void rewards = %d, want exactly 50", n)
	}
	qualified := count(t, pool, `SELECT count(*) FROM referrals WHERE referee_id IN ('bob','carol') AND status = 'QUALIFIED'`)
	capped := count(t, pool, `SELECT count(*) FROM referrals WHERE referee_id IN ('bob','carol') AND status = 'CAPPED'`)
	if qualified != 1 || capped != 1 {
		t.Fatalf("qualified=%d capped=%d, want 1 and 1", qualified, capped)
	}
}

func dueReward(t *testing.T, r store.ReferralRepository, referrer, code, referee, order string) {
	t.Helper()
	if _, err := r.CreateCode(context.Background(), referrer, code); err != nil {
		t.Fatal(err)
	}
	signup(t, r, referee, code, t0)
	complete(t, r, referee, order, t0.Add(time.Hour), t0, 50)
}

func TestReleaseAndVoidAreMutuallyExclusive(t *testing.T) {
	ctx := context.Background()
	pool := schemaPool(t)
	r := store.NewPostgresReferralRepo(pool)
	now := t0.Add(2 * time.Hour)

	for i := 0; i < 20; i++ {
		referrer, referee, order := fmt.Sprintf("a%d", i), fmt.Sprintf("b%d", i), fmt.Sprintf("o%d", i)
		dueReward(t, r, referrer, fmt.Sprintf("REF%08d", i), referee, order)

		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, _ = r.ReleaseDue(ctx, releaseIn(now, 5, 50))
		}()
		go func() {
			defer wg.Done()
			<-start
			_ = r.ApplyReturnRequested(ctx, store.ReturnRequestedInput{EventID: "rr-" + order, OrderID: order, At: now})
		}()
		close(start)
		wg.Wait()

		rw, err := r.GetRewardByOrder(ctx, order)
		if err != nil {
			t.Fatal(err)
		}
		ref, err := r.GetReferral(ctx, referee)
		if err != nil {
			t.Fatal(err)
		}
		released := count(t, pool, `SELECT count(*) FROM referral_outbox_events WHERE event_type = $1 AND aggregate_id = $2`, typeReleased, referee)
		switch rw.Status {
		case store.RewardReleased:
			if released != 1 || ref.Status != store.StatusRewarded {
				t.Fatalf("iteration %d: RELEASED with %d outbox rows, referral %s", i, released, ref.Status)
			}
		case store.RewardVoid:
			if released != 0 || ref.Status != store.StatusAttributed {
				t.Fatalf("iteration %d: VOID with %d release rows, referral %s", i, released, ref.Status)
			}
		default:
			t.Fatalf("iteration %d: reward ended %s", i, rw.Status)
		}
	}
}

func TestConcurrentSweepReleasesOnce(t *testing.T) {
	ctx := context.Background()
	pool := schemaPool(t)
	r := store.NewPostgresReferralRepo(pool)
	dueReward(t, r, "alice", "REFAAAA1111", "bob", "o1")
	now := t0.Add(2 * time.Hour)

	var wg sync.WaitGroup
	start := make(chan struct{})
	var mu sync.Mutex
	total := 0
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			n, _ := r.ReleaseDue(ctx, releaseIn(now, 5, 50))
			mu.Lock()
			total += n
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()

	if total != 1 {
		t.Fatalf("sweeps reported %d releases, want 1", total)
	}
	if n := count(t, pool, `SELECT count(*) FROM referral_outbox_events WHERE event_type = $1`, typeReleased); n != 1 {
		t.Fatalf("ReferralRewardReleased outbox rows = %d, want 1", n)
	}
}

func TestReleaseRollsBackWithOutbox(t *testing.T) {
	ctx := context.Background()
	pool := schemaPool(t)
	r := store.NewPostgresReferralRepo(pool)
	dueReward(t, r, "alice", "REFAAAA1111", "bob", "o1")
	now := t0.Add(2 * time.Hour)

	installOutboxFailure(t, pool, "WHEN (NEW.event_type = '"+typeReleased+"')")
	_, _ = r.ReleaseDue(ctx, releaseIn(now, 5, 50))

	rw, err := r.GetRewardByOrder(ctx, "o1")
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := r.GetReferral(ctx, "bob")
	if rw.Status != store.RewardPending || rw.ReleasedAt != nil || ref.Status != store.StatusQualified {
		t.Fatalf("after failed release: reward %s released_at=%v referral %s", rw.Status, rw.ReleasedAt, ref.Status)
	}

	removeOutboxFailure(t, pool)
	n, err := r.ReleaseDue(ctx, releaseIn(now.Add(time.Minute), 5, 50))
	if err != nil || n != 1 {
		t.Fatalf("retry sweep = %d, %v; want 1 release", n, err)
	}
	if rw, _ = r.GetRewardByOrder(ctx, "o1"); rw.Status != store.RewardReleased {
		t.Fatalf("reward after retry = %s, want RELEASED", rw.Status)
	}
}

func TestMigration002BackfillsLegacyRewards(t *testing.T) {
	ctx := context.Background()
	url := testkitSchemaURL(t)
	legacy := fstest.MapFS{}
	for _, name := range []string{"001_create_referrals.up.sql", "001_create_referrals.down.sql"} {
		b, err := fs.ReadFile(migrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		legacy[name] = &fstest.MapFile{Data: b}
	}
	if err := migrate.Up(ctx, url, legacy); err != nil {
		t.Fatalf("apply 001: %v", err)
	}
	pool := openPool(t, url)
	created := time.Date(2026, 9, 1, 8, 30, 0, 0, time.UTC)
	for _, q := range []string{
		`INSERT INTO referral_codes (user_id, code) VALUES ('alice', 'REFAAAA1111')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO referrals (referrer_id, referee_id, code, created_at) VALUES ('alice', 'bob', 'REFAAAA1111', $1)`, created); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO referral_rewards (id, user_id, amount, reason, created_at) VALUES ('rwd_legacy1', 'alice', 10000, 'invitee_joined', $1)`, created); err != nil {
		t.Fatal(err)
	}

	if err := migrate.Up(ctx, url, migrations.Files); err != nil {
		t.Fatalf("apply 002: %v", err)
	}

	var status string
	var releasedAt, releaseAt, createdAt time.Time
	err := pool.QueryRow(ctx, `SELECT status, released_at, release_at, created_at FROM referral_rewards WHERE id = 'rwd_legacy1'`).
		Scan(&status, &releasedAt, &releaseAt, &createdAt)
	if err != nil {
		t.Fatalf("read reward: %v", err)
	}
	if status != "RELEASED" || !releasedAt.Equal(createdAt) || !releaseAt.Equal(createdAt) || !createdAt.Equal(created) {
		t.Fatalf("legacy reward = %s released_at=%v release_at=%v created_at=%v", status, releasedAt, releaseAt, createdAt)
	}
	var refStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM referrals WHERE referee_id = 'bob'`).Scan(&refStatus); err != nil || refStatus != "REWARDED" {
		t.Fatalf("legacy referral status = %q, %v; want REWARDED", refStatus, err)
	}

	r := store.NewPostgresReferralRepo(pool)
	tot, err := r.RewardTotals(ctx, "alice")
	if err != nil || tot.ReleasedXU != 10000 || tot.PendingXU != 0 {
		t.Fatalf("totals after migration = %+v, %v", tot, err)
	}
	for _, idx := range []struct{ table, cols string }{
		{"referral_rewards", "status, release_at"},
		{"referral_rewards", "user_id, released_at"},
	} {
		var n int
		err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE tablename = $1 AND indexdef LIKE '%('||$2||')%'`, idx.table, idx.cols).Scan(&n)
		if err != nil || n == 0 {
			t.Errorf("no index on %s (%s): n=%d err=%v", idx.table, idx.cols, n, err)
		}
	}
}

func TestProcessedEventsOlderThanSevenDaysArePurged(t *testing.T) {
	ctx := context.Background()
	pool := schemaPool(t)
	r := store.NewPostgresReferralRepo(pool)
	now := t0.Add(30 * 24 * time.Hour)
	for id, age := range map[string]time.Duration{"old": 8 * 24 * time.Hour, "fresh": 6 * 24 * time.Hour} {
		err := r.ApplyUserRegistered(ctx, store.UserRegisteredInput{EventID: id, UserID: "u-" + id, RegisteredAt: now.Add(-age), At: now.Add(-age), LifetimeCap: 50})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.PurgeProcessedEvents(ctx, now.Add(-7*24*time.Hour)); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n := count(t, pool, `SELECT count(*) FROM referral_processed_events WHERE event_id = 'old'`); n != 0 {
		t.Errorf("old processed event survived the purge")
	}
	if n := count(t, pool, `SELECT count(*) FROM referral_processed_events WHERE event_id = 'fresh'`); n != 1 {
		t.Errorf("fresh processed event was purged")
	}
}
