package store_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/buidangphuc/team-promotion/internal/store"
)

const migration9 = "0009_plan_billing.up.sql"

type planRow struct {
	ID       string
	Tier     int32
	Price    int64
	TermDays int32
}

func TestMigrationCorrectsPlanPrices_CatalogShowsCorrectedPrices(t *testing.T) {
	pool := newPoolUpTo(t, 8)
	ctx := t.Context()
	if _, err := pool.Exec(ctx, migrationSQL(t, migration9)); err != nil {
		t.Fatalf("apply %s: %v", migration9, err)
	}
	rows, err := pool.Query(ctx, `SELECT id, tier, price, term_days FROM subscription_plans ORDER BY tier`)
	if err != nil {
		t.Fatalf("read plans: %v", err)
	}
	defer rows.Close()
	var got []planRow
	for rows.Next() {
		var p planRow
		if err := rows.Scan(&p.ID, &p.Tier, &p.Price, &p.TermDays); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, p)
	}
	want := []planRow{{"plan_free", 1, 0, 30}, {"plan_pro", 2, 199000, 30}, {"plan_premium", 3, 499000, 30}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("catalog after migration = %+v, want %+v", got, want)
	}
}

func TestMigrationCorrectsPlanPrices_ListPlansServesCorrectedPrices(t *testing.T) {
	pool := newTestPool(t)
	plans, err := store.NewPostgresSubscriptionRepository(pool).ListPlans(t.Context())
	if err != nil {
		t.Fatalf("ListPlans: %v", err)
	}
	var got []planRow
	for _, p := range plans {
		got = append(got, planRow{p.ID, p.Tier, p.Price, p.TermDays})
	}
	want := []planRow{{"plan_free", 1, 0, 30}, {"plan_pro", 2, 199000, 30}, {"plan_premium", 3, 499000, 30}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListPlans = %+v, want %+v", got, want)
	}
}

func TestDefaultPlansMatchMigratedCatalog_GoCatalogAndMigratedRowsAgree(t *testing.T) {
	pool := newTestPool(t)
	ctx := t.Context()
	for _, want := range store.DefaultPlans() {
		var tier int32
		var price int64
		var term int32
		var limits map[string]string
		err := pool.QueryRow(ctx, `SELECT tier, price, term_days, limits FROM subscription_plans WHERE id = $1`, want.ID).
			Scan(&tier, &price, &term, &limits)
		if err != nil {
			t.Fatalf("read plan %s: %v", want.ID, err)
		}
		if tier != want.Tier || price != want.Price || term != want.TermDays {
			t.Fatalf("plan %s row (tier %d price %d term %d) != DefaultPlans (tier %d price %d term %d)",
				want.ID, tier, price, term, want.Tier, want.Price, want.TermDays)
		}
		if !reflect.DeepEqual(limits, want.Limits) {
			t.Fatalf("plan %s limits %v != DefaultPlans %v", want.ID, limits, want.Limits)
		}
	}
	if want := []int64{0, 199000, 499000}; true {
		var got []int64
		for _, p := range store.DefaultPlans() {
			got = append(got, p.Price)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("DefaultPlans prices = %v, want %v", got, want)
		}
	}
}

func TestPlanTermDaysMustBePositive_SchemaRejectsZero(t *testing.T) {
	pool := newTestPool(t)
	ctx := t.Context()

	_, err := pool.Exec(ctx, `INSERT INTO subscription_plans (id, tier, price, term_days) VALUES ('plan_bad', 9, 1, 0)`)
	assertCheckViolation(t, err, "insert term_days 0")

	_, err = pool.Exec(ctx, `UPDATE subscription_plans SET term_days = 0 WHERE id = 'plan_pro'`)
	assertCheckViolation(t, err, "update term_days 0")

	var term int32
	if err := pool.QueryRow(ctx, `SELECT term_days FROM subscription_plans WHERE id = 'plan_pro'`).Scan(&term); err != nil || term != 30 {
		t.Fatalf("plan_pro term_days = %d err=%v, want 30 unchanged", term, err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM subscription_plans`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("plan count = %d err=%v, want 3", n, err)
	}
}

func assertCheckViolation(t *testing.T, err error, what string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("%s: err = %v, want check_violation 23514", what, err)
	}
}

func TestMigrationGrandfathersActiveSubscriptions_ActivePaidRowGetsOneTerm(t *testing.T) {
	pool := newPoolUpTo(t, 8)
	ctx := t.Context()
	if _, err := pool.Exec(ctx, `INSERT INTO seller_subscriptions (id, seller_id, plan_id, status) VALUES
		('sub-pro', 'seller-pro', 'plan_pro', 'active'),
		('sub-old', 'seller-old', 'plan_pro', 'expired')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var before time.Time
	if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&before); err != nil {
		t.Fatalf("clock: %v", err)
	}
	if _, err := pool.Exec(ctx, migrationSQL(t, migration9)); err != nil {
		t.Fatalf("apply %s: %v", migration9, err)
	}
	var after time.Time
	if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&after); err != nil {
		t.Fatalf("clock: %v", err)
	}

	var end time.Time
	var autoRenew bool
	var planID, status string
	if err := pool.QueryRow(ctx, `SELECT current_period_end, auto_renew, plan_id, status FROM seller_subscriptions WHERE id = 'sub-pro'`).
		Scan(&end, &autoRenew, &planID, &status); err != nil {
		t.Fatalf("read grandfathered row: %v", err)
	}
	lo, hi := before.Add(30*24*time.Hour), after.Add(30*24*time.Hour)
	if end.Before(lo) || end.After(hi) {
		t.Fatalf("current_period_end = %v, want within [%v, %v] (30 days after migration)", end, lo, hi)
	}
	if !autoRenew || planID != "plan_pro" || status != "active" {
		t.Fatalf("row = auto_renew %v plan %s status %s, want true plan_pro active", autoRenew, planID, status)
	}

	var oldEnd *time.Time
	if err := pool.QueryRow(ctx, `SELECT current_period_end FROM seller_subscriptions WHERE id = 'sub-old'`).Scan(&oldEnd); err != nil {
		t.Fatalf("read inactive row: %v", err)
	}
	if oldEnd != nil {
		t.Fatalf("expired row got current_period_end %v, want untouched (NULL)", *oldEnd)
	}
}

func TestConcurrentSubscribeSingleCharge_SubscriptionRowTwoCallsChargeOnce(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresSubscriptionRepository(pool)
	var charges atomic.Int32
	now := time.Now().UTC().Truncate(time.Second)

	subscribe := func(ctx context.Context) (store.SellerSubscription, error) {
		var out store.SellerSubscription
		err := repo.WithSellerLock(ctx, "seller-1", func(ctx context.Context, tx store.SubscriptionTx) error {
			cur, err := tx.Get(ctx)
			if err != nil {
				return err
			}
			if cur.PlanID == "plan_pro" && cur.Status == store.SubscriptionStatusActive {
				out = cur
				return nil
			}
			charges.Add(1)
			out, err = tx.UpdatePeriod(ctx, store.SubscriptionPeriod{
				PlanID: "plan_pro", Status: store.SubscriptionStatusActive,
				CurrentPeriodStart: now, CurrentPeriodEnd: now.Add(30 * 24 * time.Hour),
				AutoRenew: true, PeriodSeq: cur.PeriodSeq + 1,
			})
			return err
		})
		return out, err
	}

	var wg sync.WaitGroup
	results := make([]store.SellerSubscription, 2)
	errs := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = subscribe(t.Context())
		}()
	}
	wg.Wait()

	for i := range results {
		if errs[i] != nil {
			t.Fatalf("subscribe %d: %v", i, errs[i])
		}
		if results[i].PlanID != "plan_pro" || results[i].Status != store.SubscriptionStatusActive {
			t.Fatalf("subscribe %d result = %+v, want ACTIVE plan_pro", i, results[i])
		}
	}
	if n := charges.Load(); n != 1 {
		t.Fatalf("charges = %d, want 1", n)
	}
	got, err := repo.GetActiveSubscriptionBySeller(t.Context(), "seller-1")
	if err != nil {
		t.Fatalf("read subscription: %v", err)
	}
	if got.PeriodSeq != 1 || !got.AutoRenew || got.PendingPlanID != "" || !got.CurrentPeriodEnd.Equal(now.Add(30*24*time.Hour)) {
		t.Fatalf("stored = %+v, want period_seq 1 auto_renew true no pending end now+30d", got)
	}
}

func TestSubscriptionLockGet_PlaceholderRowForNewSeller(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresSubscriptionRepository(pool)
	err := repo.WithSellerLock(t.Context(), "seller-new", func(ctx context.Context, tx store.SubscriptionTx) error {
		cur, err := tx.Get(ctx)
		if err != nil {
			return err
		}
		if cur.SellerID != "seller-new" || cur.ID == "" || cur.PeriodSeq != 0 || cur.Status == store.SubscriptionStatusActive {
			t.Fatalf("placeholder = %+v, want generated id, period_seq 0, not active", cur)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithSellerLock: %v", err)
	}
	if _, err := repo.GetActiveSubscriptionBySeller(t.Context(), "seller-new"); !errors.Is(err, store.ErrSubscriptionNotFound) {
		t.Fatalf("active lookup after placeholder = %v, want ErrSubscriptionNotFound", err)
	}
}

func TestSubscriptionLockRollsBackOnError_NoPeriodWritten(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresSubscriptionRepository(pool)
	boom := errors.New("charge failed")
	now := time.Now().UTC().Truncate(time.Second)
	err := repo.WithSellerLock(t.Context(), "seller-2", func(ctx context.Context, tx store.SubscriptionTx) error {
		if _, err := tx.UpdatePeriod(ctx, store.SubscriptionPeriod{
			PlanID: "plan_pro", Status: store.SubscriptionStatusActive,
			CurrentPeriodStart: now, CurrentPeriodEnd: now.Add(time.Hour), AutoRenew: true, PeriodSeq: 1,
		}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("WithSellerLock err = %v, want %v", err, boom)
	}
	if _, err := repo.GetActiveSubscriptionBySeller(t.Context(), "seller-2"); !errors.Is(err, store.ErrSubscriptionNotFound) {
		t.Fatalf("active lookup after rollback = %v, want ErrSubscriptionNotFound", err)
	}
}

func TestListDueForRenewal_SelectsOnlyDueActiveRows(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresSubscriptionRepository(pool)
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx, `INSERT INTO seller_subscriptions
		(id, seller_id, plan_id, status, current_period_start, current_period_end, auto_renew) VALUES
		('s-due',    'a', 'plan_pro', 'active',  $1, $2, true),
		('s-edge',   'b', 'plan_pro', 'active',  $1, $3, true),
		('s-future', 'c', 'plan_pro', 'active',  $1, $4, true),
		('s-lapsed', 'd', 'plan_pro', 'expired', $1, $2, true)`,
		now.Add(-31*24*time.Hour), now.Add(-time.Hour), now, now.Add(time.Hour)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, err := repo.ListDueForRenewal(ctx, now, 10)
	if err != nil {
		t.Fatalf("ListDueForRenewal: %v", err)
	}
	var ids []string
	for _, s := range got {
		ids = append(ids, s.ID)
	}
	if !reflect.DeepEqual(ids, []string{"s-due", "s-edge"}) {
		t.Fatalf("due ids = %v, want [s-due s-edge] oldest first, boundary inclusive", ids)
	}
	limited, err := repo.ListDueForRenewal(ctx, now, 1)
	if err != nil || len(limited) != 1 || limited[0].ID != "s-due" {
		t.Fatalf("limit 1 = %+v err=%v, want only s-due", limited, err)
	}
}

func TestListDueForRenewal_SkipsRowLockedBySubscribe(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresSubscriptionRepository(pool)
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx, `INSERT INTO seller_subscriptions
		(id, seller_id, plan_id, status, current_period_start, current_period_end, auto_renew) VALUES
		('s-locked', 'a', 'plan_pro', 'active', $1, $2, true),
		('s-free',   'b', 'plan_pro', 'active', $1, $2, true)`,
		now.Add(-31*24*time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := repo.WithSellerLock(ctx, "a", func(ctx context.Context, tx store.SubscriptionTx) error {
		if _, err := tx.Get(ctx); err != nil {
			return err
		}
		got, err := repo.ListDueForRenewal(ctx, now, 10)
		if err != nil {
			return err
		}
		if len(got) != 1 || got[0].ID != "s-free" {
			t.Fatalf("due while seller a locked = %+v, want only s-free (SKIP LOCKED)", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithSellerLock: %v", err)
	}
}
