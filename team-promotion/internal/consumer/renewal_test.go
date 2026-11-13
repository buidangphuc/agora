package consumer_test

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/team-promotion/internal/consumer"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
	"github.com/buidangphuc/team-promotion/internal/subscription"
	"github.com/buidangphuc/team-promotion/internal/subscription/paymentfake"
)

var renewT0 = time.Date(2026, 12, 13, 9, 0, 0, 0, time.UTC)

const (
	renewDay    = 24 * time.Hour
	retryWindow = time.Hour
)

type renewEnv struct {
	repo  subscription.Repository
	pay   *paymentfake.Charger
	sw    *consumer.RenewalSweeper
	svc   *subscription.Service
	clock time.Time
}

func newRenewEnvOn(t *testing.T, repo subscription.Repository) *renewEnv {
	t.Helper()
	e := &renewEnv{repo: repo, pay: paymentfake.New(), clock: renewT0}
	e.sw = consumer.NewRenewalSweeper(repo, e.pay, consumer.RenewalConfig{
		RetryWindow: retryWindow,
		Now:         func() time.Time { return e.clock },
	})
	e.svc = subscription.New(repo, nil).WithPayment(e.pay)
	return e
}

func newRenewEnv(t *testing.T) *renewEnv {
	t.Helper()
	return newRenewEnvOn(t, storefake.NewInMemorySubscriptionRepository())
}

type dueRow struct {
	plan      string
	autoRenew bool
	pending   string
}

func (e *renewEnv) seedDue(t *testing.T, seller string, r dueRow) store.SellerSubscription {
	t.Helper()
	var out store.SellerSubscription
	err := e.repo.WithSellerLock(t.Context(), seller, func(ctx context.Context, tx store.SubscriptionTx) error {
		if _, err := tx.Get(ctx); err != nil {
			return err
		}
		var err error
		out, err = tx.UpdatePeriod(ctx, store.SubscriptionPeriod{
			PlanID: r.plan, Status: store.SubscriptionStatusActive,
			CurrentPeriodStart: renewT0.Add(-30 * renewDay), CurrentPeriodEnd: renewT0,
			AutoRenew: r.autoRenew, PendingPlanID: r.pending, PeriodSeq: 1,
		})
		return err
	})
	if err != nil {
		t.Fatalf("seed %s: %v", seller, err)
	}
	return out
}

func (e *renewEnv) stored(t *testing.T, seller string) store.SellerSubscription {
	t.Helper()
	var out store.SellerSubscription
	err := e.repo.WithSellerLock(t.Context(), seller, func(ctx context.Context, tx store.SubscriptionTx) error {
		var err error
		out, err = tx.Get(ctx)
		return err
	})
	if err != nil {
		t.Fatalf("read %s: %v", seller, err)
	}
	return out
}

func (e *renewEnv) sweep(t *testing.T) {
	t.Helper()
	if _, err := e.sw.Sweep(t.Context()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
}

func (e *renewEnv) tier(t *testing.T, seller string) int32 {
	t.Helper()
	tier, _, err := e.svc.GetEntitlements(t.Context(), seller)
	if err != nil {
		t.Fatalf("GetEntitlements: %v", err)
	}
	return tier
}

func TestSweepRenewsOncePerPeriod(t *testing.T) {
	// Scenario: Auto-renew charges one fee per period
	e := newRenewEnv(t)
	row := e.seedDue(t, "seller-1", dueRow{plan: "plan_pro", autoRenew: true})
	e.clock = renewT0.Add(2 * time.Minute)

	e.sweep(t)
	e.sweep(t)

	fees := e.pay.Fees()
	if len(fees) != 1 || fees[0].Amount != 199000 || fees[0].SellerID != "seller-1" {
		t.Fatalf("fees = %+v, want exactly one of 199000 for seller-1", fees)
	}
	if want := fmt.Sprintf("sub:%s:2:plan_pro", row.ID); fees[0].ReferenceID != want {
		t.Fatalf("key = %q, want %q", fees[0].ReferenceID, want)
	}
	if n := len(e.pay.Calls()); n != 1 {
		t.Fatalf("charge calls over two sweeps = %d, want 1", n)
	}
	got := e.stored(t, "seller-1")
	if got.Status != store.SubscriptionStatusActive || got.PlanID != "plan_pro" || !got.AutoRenew {
		t.Fatalf("stored = %+v, want ACTIVE plan_pro auto_renew", got)
	}
	if !got.CurrentPeriodStart.Equal(e.clock) || !got.CurrentPeriodEnd.Equal(e.clock.Add(30*renewDay)) || got.PeriodSeq != 2 {
		t.Fatalf("period = %v..%v seq %d, want renewal instant..+30d seq 2", got.CurrentPeriodStart, got.CurrentPeriodEnd, got.PeriodSeq)
	}
}

func TestSweepIgnoresSubscriptionsNotYetDue(t *testing.T) {
	// Scenario: Auto-renew charges one fee per period
	e := newRenewEnv(t)
	e.seedDue(t, "seller-1", dueRow{plan: "plan_pro", autoRenew: true})
	e.clock = renewT0.Add(-time.Second)
	before := e.stored(t, "seller-1")

	e.sweep(t)

	if n := len(e.pay.Calls()); n != 0 {
		t.Fatalf("charge calls = %d, want 0 before the period end", n)
	}
	if after := e.stored(t, "seller-1"); !reflect.DeepEqual(before, after) {
		t.Fatalf("row changed before due:\nbefore %+v\nafter  %+v", before, after)
	}
}

func TestSweepCancelledLapsesToFree(t *testing.T) {
	// Scenario: Cancelled subscription lapses to FREE
	e := newRenewEnv(t)
	e.seedDue(t, "seller-1", dueRow{plan: "plan_pro", autoRenew: false})
	e.clock = renewT0.Add(time.Minute)

	e.sweep(t)

	if n := len(e.pay.Calls()); n != 0 {
		t.Fatalf("charge calls = %d, want 0", n)
	}
	if got := e.tier(t, "seller-1"); got != store.PlanTierFree {
		t.Fatalf("tier = %d, want FREE", got)
	}
	if got := e.stored(t, "seller-1").Status; got != store.SubscriptionStatusCancelled {
		t.Fatalf("status = %q, want cancelled", got)
	}
}

func TestSweepAppliesPendingDowngrade(t *testing.T) {
	// Scenario: Pending downgrade renews at the lower price
	e := newRenewEnv(t)
	row := e.seedDue(t, "seller-1", dueRow{plan: "plan_premium", autoRenew: true, pending: "plan_pro"})
	e.clock = renewT0.Add(time.Minute)

	e.sweep(t)

	calls := e.pay.Calls()
	if len(calls) != 1 || calls[0].Amount != 199000 {
		t.Fatalf("charges = %+v, want one of 199000", calls)
	}
	if want := fmt.Sprintf("sub:%s:2:plan_pro", row.ID); calls[0].ReferenceID != want {
		t.Fatalf("key = %q, want %q", calls[0].ReferenceID, want)
	}
	got := e.stored(t, "seller-1")
	if got.PlanID != "plan_pro" || got.PendingPlanID != "" || got.Status != store.SubscriptionStatusActive {
		t.Fatalf("stored = %+v, want ACTIVE plan_pro with no pending", got)
	}
}

func TestSweepPendingFreeLapsesExpiredWithoutCharge(t *testing.T) {
	// Scenario: Failed renewal charge sets the seller to FREE
	e := newRenewEnv(t)
	e.seedDue(t, "seller-1", dueRow{plan: "plan_pro", autoRenew: true, pending: "plan_free"})
	e.clock = renewT0.Add(time.Minute)

	e.sweep(t)

	if n := len(e.pay.Calls()); n != 0 {
		t.Fatalf("charge calls = %d, want 0 for a FREE target", n)
	}
	if got := e.stored(t, "seller-1").Status; got != store.SubscriptionStatusExpired {
		t.Fatalf("status = %q, want expired", got)
	}
	if got := e.tier(t, "seller-1"); got != store.PlanTierFree {
		t.Fatalf("tier = %d, want FREE", got)
	}
}

func TestSweepInsufficientBalanceLapsesToFree(t *testing.T) {
	// Scenario: Failed renewal charge sets the seller to FREE
	e := newRenewEnv(t)
	e.seedDue(t, "seller-1", dueRow{plan: "plan_pro", autoRenew: true})
	e.clock = renewT0.Add(time.Minute)
	e.pay.SetError(status.Error(codes.FailedPrecondition, "insufficient withdrawable balance"))

	e.sweep(t)

	if got := e.tier(t, "seller-1"); got != store.PlanTierFree {
		t.Fatalf("tier = %d, want FREE", got)
	}
	if got := e.stored(t, "seller-1").Status; got != store.SubscriptionStatusExpired {
		t.Fatalf("status = %q, want expired", got)
	}
	if n := len(e.pay.Fees()); n != 0 {
		t.Fatalf("fees = %d, want 0", n)
	}
}

func TestSweepTransientErrorRetriesWithinWindow(t *testing.T) {
	// Scenario: Transient payment error is retried inside the window
	e := newRenewEnv(t)
	e.seedDue(t, "seller-1", dueRow{plan: "plan_pro", autoRenew: true})
	before := e.stored(t, "seller-1")
	e.clock = renewT0.Add(10 * time.Minute)
	e.pay.SetError(status.Error(codes.Unavailable, "down"))

	e.sweep(t)

	if after := e.stored(t, "seller-1"); !reflect.DeepEqual(before, after) {
		t.Fatalf("row changed on a transient error:\nbefore %+v\nafter  %+v", before, after)
	}
	if got := e.tier(t, "seller-1"); got != store.PlanTierPro {
		t.Fatalf("tier = %d, want PRO while retrying", got)
	}

	e.pay.SetError(nil)
	e.clock = renewT0.Add(11 * time.Minute)
	e.sweep(t)

	calls := e.pay.Calls()
	if len(calls) != 2 || calls[0].ReferenceID != calls[1].ReferenceID {
		t.Fatalf("calls = %+v, want two with the same key", calls)
	}
	got := e.stored(t, "seller-1")
	if got.Status != store.SubscriptionStatusActive || got.PeriodSeq != 2 || !got.CurrentPeriodEnd.Equal(e.clock.Add(30*renewDay)) {
		t.Fatalf("stored = %+v, want renewed ACTIVE seq 2", got)
	}
}

func TestSweepTransientErrorPastWindowLapses(t *testing.T) {
	// Scenario: Retry window exhausted lapses the seller
	e := newRenewEnv(t)
	e.seedDue(t, "seller-1", dueRow{plan: "plan_pro", autoRenew: true})
	e.pay.SetError(status.Error(codes.Unavailable, "down"))

	e.clock = renewT0.Add(retryWindow - time.Minute)
	e.sweep(t)
	if got := e.stored(t, "seller-1").Status; got != store.SubscriptionStatusActive {
		t.Fatalf("status at 59m = %q, want still active", got)
	}

	e.clock = renewT0.Add(retryWindow + time.Minute)
	e.sweep(t)

	if got := e.stored(t, "seller-1").Status; got != store.SubscriptionStatusExpired {
		t.Fatalf("status at 61m = %q, want expired", got)
	}
	if got := e.tier(t, "seller-1"); got != store.PlanTierFree {
		t.Fatalf("tier = %d, want FREE", got)
	}
}

func overdueGauge(t *testing.T, r *sdkmetric.ManualReader) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := r.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "subscription_renewals_overdue" {
				continue
			}
			g, ok := m.Data.(metricdata.Gauge[int64])
			if !ok || len(g.DataPoints) != 1 {
				t.Fatalf("subscription_renewals_overdue: unexpected data %#v", m.Data)
			}
			return g.DataPoints[0].Value
		}
	}
	t.Fatal("subscription_renewals_overdue not reported")
	return 0
}

func TestSweepReportsOverdueRenewalsMetric(t *testing.T) {
	// Scenario: Two sweepers do not double charge
	prev := otel.GetMeterProvider()
	reader := sdkmetric.NewManualReader()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	t.Cleanup(func() { otel.SetMeterProvider(prev) })

	e := newRenewEnv(t)
	e.seedDue(t, "seller-1", dueRow{plan: "plan_pro", autoRenew: true})
	e.seedDue(t, "seller-2", dueRow{plan: "plan_pro", autoRenew: true})
	e.clock = renewT0.Add(10 * time.Minute)
	e.pay.SetError(status.Error(codes.Unavailable, "down"))

	e.sweep(t)
	if got := overdueGauge(t, reader); got != 2 {
		t.Fatalf("subscription_renewals_overdue = %d, want 2 while charges fail", got)
	}

	e.pay.SetError(nil)
	e.sweep(t)
	if got := overdueGauge(t, reader); got != 0 {
		t.Fatalf("subscription_renewals_overdue = %d, want 0 after renewals", got)
	}
}

func TestSweepConcurrentSweepersSingleCharge(t *testing.T) {
	// Scenario: Two sweepers do not double charge
	pool := newPool(t)
	repo := store.NewPostgresSubscriptionRepository(pool)
	e := newRenewEnvOn(t, repo)
	row := e.seedDue(t, "seller-1", dueRow{plan: "plan_pro", autoRenew: true})
	e.clock = renewT0.Add(time.Minute)
	other := consumer.NewRenewalSweeper(repo, e.pay, consumer.RenewalConfig{
		RetryWindow: retryWindow,
		Now:         func() time.Time { return e.clock },
	})

	var wg sync.WaitGroup
	errs := make([]error, 2)
	start := make(chan struct{})
	for i, sw := range []*consumer.RenewalSweeper{e.sw, other} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = sw.Sweep(t.Context())
		}()
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("sweeper %d: %v", i, err)
		}
	}

	if n := len(e.pay.Calls()); n != 1 {
		t.Fatalf("charge calls = %d, want 1 across two sweepers", n)
	}
	fees := e.pay.Fees()
	if want := fmt.Sprintf("sub:%s:2:plan_pro", row.ID); len(fees) != 1 || fees[0].ReferenceID != want {
		t.Fatalf("fees = %+v, want one with key %q", fees, want)
	}
	if got := e.stored(t, "seller-1"); got.PeriodSeq != 2 || got.Status != store.SubscriptionStatusActive {
		t.Fatalf("stored = %+v, want ACTIVE seq 2", got)
	}
}
