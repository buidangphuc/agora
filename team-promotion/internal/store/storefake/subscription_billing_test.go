package storefake_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
)

func TestPlanCatalog_FakeServesCorrectedPrices(t *testing.T) {
	plans, err := storefake.NewInMemorySubscriptionRepository().ListPlans(t.Context())
	if err != nil {
		t.Fatalf("ListPlans: %v", err)
	}
	type row struct {
		ID    string
		Price int64
		Term  int32
	}
	var got []row
	for _, p := range plans {
		got = append(got, row{p.ID, p.Price, p.TermDays})
	}
	want := []row{{"plan_free", 0, 30}, {"plan_pro", 199000, 30}, {"plan_premium", 499000, 30}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("catalog = %+v, want %+v", got, want)
	}
}

func TestPlanCatalog_FakeEqualsDefaultPlans(t *testing.T) {
	plans, err := storefake.NewInMemorySubscriptionRepository().ListPlans(t.Context())
	if err != nil {
		t.Fatalf("ListPlans: %v", err)
	}
	if !reflect.DeepEqual(plans, store.DefaultPlans()) {
		t.Fatalf("fake catalog %+v != DefaultPlans %+v", plans, store.DefaultPlans())
	}
	for _, p := range plans {
		if p.TermDays <= 0 {
			t.Fatalf("plan %s TermDays = %d, want positive", p.ID, p.TermDays)
		}
	}
}

func TestConcurrentSubscribeSingleCharge_SubscriptionRowFakeTwoCallsChargeOnce(t *testing.T) {
	repo := storefake.NewInMemorySubscriptionRepository()
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
	res := make([]store.SellerSubscription, 2)
	errs := make([]error, 2)
	for i := range res {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res[i], errs[i] = subscribe(t.Context())
		}()
	}
	wg.Wait()
	for i := range res {
		if errs[i] != nil || res[i].PlanID != "plan_pro" || res[i].Status != store.SubscriptionStatusActive {
			t.Fatalf("subscribe %d = %+v err=%v, want ACTIVE plan_pro", i, res[i], errs[i])
		}
	}
	if n := charges.Load(); n != 1 {
		t.Fatalf("charges = %d, want 1", n)
	}
}

func TestSubscriptionLock_FakeRollsBackOnError(t *testing.T) {
	repo := storefake.NewInMemorySubscriptionRepository()
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
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if _, err := repo.GetActiveSubscriptionBySeller(t.Context(), "seller-2"); !errors.Is(err, store.ErrSubscriptionNotFound) {
		t.Fatalf("active lookup after rollback = %v, want ErrSubscriptionNotFound", err)
	}
}

func TestListDueForRenewal_FakeSelectsOnlyDueActiveRows(t *testing.T) {
	repo := storefake.NewInMemorySubscriptionRepository()
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Second)
	put := func(seller, status string, end time.Time) {
		err := repo.WithSellerLock(ctx, seller, func(ctx context.Context, tx store.SubscriptionTx) error {
			cur, err := tx.Get(ctx)
			if err != nil {
				return err
			}
			_, err = tx.UpdatePeriod(ctx, store.SubscriptionPeriod{
				PlanID: "plan_pro", Status: status,
				CurrentPeriodStart: end.Add(-30 * 24 * time.Hour), CurrentPeriodEnd: end,
				AutoRenew: true, PeriodSeq: cur.PeriodSeq + 1,
			})
			return err
		})
		if err != nil {
			t.Fatalf("seed %s: %v", seller, err)
		}
	}
	put("due", store.SubscriptionStatusActive, now.Add(-time.Hour))
	put("edge", store.SubscriptionStatusActive, now)
	put("future", store.SubscriptionStatusActive, now.Add(time.Hour))
	put("lapsed", store.SubscriptionStatusExpired, now.Add(-time.Hour))

	got, err := repo.ListDueForRenewal(ctx, now, 10)
	if err != nil {
		t.Fatalf("ListDueForRenewal: %v", err)
	}
	var sellers []string
	for _, s := range got {
		sellers = append(sellers, s.SellerID)
	}
	if !reflect.DeepEqual(sellers, []string{"due", "edge"}) {
		t.Fatalf("due sellers = %v, want [due edge]", sellers)
	}
}
