package subscription

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
	"github.com/buidangphuc/team-promotion/internal/subscription/paymentfake"
)

var billingT0 = time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)

const day = 24 * time.Hour

type billingFixture struct {
	svc  *Service
	repo *storefake.InMemorySubscriptionRepository
	pay  *paymentfake.Charger
	now  time.Time
}

func newBilling(t *testing.T) *billingFixture {
	t.Helper()
	f := &billingFixture{
		repo: storefake.NewInMemorySubscriptionRepository(),
		pay:  paymentfake.New(),
		now:  billingT0,
	}
	f.svc = New(f.repo, nil).WithPayment(f.pay)
	f.svc.nowFn = func() time.Time { return f.now }
	return f
}

type seedRow struct {
	plan      string
	status    string
	start     time.Time
	end       time.Time
	autoRenew bool
	pending   string
	seq       int64
}

func seed(t *testing.T, repo Repository, seller string, r seedRow) store.SellerSubscription {
	t.Helper()
	var out store.SellerSubscription
	err := repo.WithSellerLock(t.Context(), seller, func(ctx context.Context, tx store.SubscriptionTx) error {
		if _, err := tx.Get(ctx); err != nil {
			return err
		}
		var err error
		out, err = tx.UpdatePeriod(ctx, store.SubscriptionPeriod{
			PlanID: r.plan, Status: r.status, CurrentPeriodStart: r.start, CurrentPeriodEnd: r.end,
			AutoRenew: r.autoRenew, PendingPlanID: r.pending, PeriodSeq: r.seq,
		})
		return err
	})
	if err != nil {
		t.Fatalf("seed %s: %v", seller, err)
	}
	return out
}

func activeRow(plan string, start, end time.Time) seedRow {
	return seedRow{plan: plan, status: store.SubscriptionStatusActive, start: start, end: end, autoRenew: true, seq: 1}
}

func stored(t *testing.T, repo Repository, seller string) store.SellerSubscription {
	t.Helper()
	var out store.SellerSubscription
	err := repo.WithSellerLock(t.Context(), seller, func(ctx context.Context, tx store.SubscriptionTx) error {
		var err error
		out, err = tx.Get(ctx)
		return err
	})
	if err != nil {
		t.Fatalf("read %s: %v", seller, err)
	}
	return out
}

func entitlementTier(t *testing.T, svc *Service, seller string) int32 {
	t.Helper()
	tier, _, err := svc.GetEntitlements(t.Context(), seller)
	if err != nil {
		t.Fatalf("GetEntitlements: %v", err)
	}
	return tier
}

func TestSubscribeUpgradeChargesAndActivates(t *testing.T) {
	// Scenario: FREE seller subscribes to PRO
	f := newBilling(t)
	sub, err := f.svc.Subscribe(t.Context(), "seller-1", "plan_pro")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	calls := f.pay.Calls()
	if len(calls) != 1 {
		t.Fatalf("ChargeSellerFee calls = %d, want exactly 1", len(calls))
	}
	wantKey := fmt.Sprintf("sub:%s:1:plan_pro", sub.ID)
	if calls[0].SellerID != "seller-1" || calls[0].Amount != 199000 || calls[0].ReferenceID != wantKey {
		t.Fatalf("charge = %+v, want seller-1 199000 %q", calls[0], wantKey)
	}
	if sub.Status != store.SubscriptionStatusActive || sub.PlanID != "plan_pro" || !sub.AutoRenew || sub.PendingPlanID != "" {
		t.Fatalf("subscription = %+v, want ACTIVE plan_pro auto_renew no pending", sub)
	}
	if !sub.CurrentPeriodStart.Equal(f.now) || !sub.CurrentPeriodEnd.Equal(f.now.Add(30*day)) {
		t.Fatalf("period = %v..%v, want %v..+30d", sub.CurrentPeriodStart, sub.CurrentPeriodEnd, f.now)
	}
	if sub.PeriodSeq != 1 {
		t.Fatalf("period_seq = %d, want 1", sub.PeriodSeq)
	}
	if got := entitlementTier(t, f.svc, "seller-1"); got != store.PlanTierPro {
		t.Fatalf("entitlement tier = %d, want PRO", got)
	}
}

func TestSubscribeInsufficientBalanceChangesNothing(t *testing.T) {
	// Scenario: Insufficient withdrawable balance is rejected
	f := newBilling(t)
	f.pay.SetError(status.Error(codes.FailedPrecondition, "insufficient withdrawable balance"))
	_, err := f.svc.Subscribe(t.Context(), "seller-1", "plan_pro")
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("err = %v, want ErrInsufficientBalance", err)
	}
	if got := entitlementTier(t, f.svc, "seller-1"); got != store.PlanTierFree {
		t.Fatalf("tier = %d, want FREE", got)
	}
	if _, err := f.repo.GetActiveSubscriptionBySeller(t.Context(), "seller-1"); !errors.Is(err, store.ErrSubscriptionNotFound) {
		t.Fatalf("active row lookup = %v, want ErrSubscriptionNotFound", err)
	}
}

func TestSubscribePaymentUnavailableChangesNothing(t *testing.T) {
	// Scenario: Payment service unavailable fails without state change
	for name, payErr := range map[string]error{
		"unavailable":       status.Error(codes.Unavailable, "down"),
		"deadline exceeded": status.Error(codes.DeadlineExceeded, "2s"),
		"internal":          status.Error(codes.Internal, "boom"),
	} {
		t.Run(name, func(t *testing.T) {
			f := newBilling(t)
			seeded := seed(t, f.repo, "seller-1", activeRow("plan_pro", f.now.Add(-5*day), f.now.Add(25*day)))
			before, err := f.repo.GetActiveSubscriptionBySeller(t.Context(), "seller-1")
			if err != nil {
				t.Fatal(err)
			}
			f.pay.SetError(payErr)
			if _, err := f.svc.Subscribe(t.Context(), "seller-1", "plan_premium"); !errors.Is(err, ErrPaymentUnavailable) {
				t.Fatalf("err = %v, want ErrPaymentUnavailable", err)
			}
			after, err := f.repo.GetActiveSubscriptionBySeller(t.Context(), "seller-1")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("subscription changed:\nbefore %+v\nafter  %+v", before, after)
			}
			if after.PlanID != "plan_pro" || after.PeriodSeq != seeded.PeriodSeq {
				t.Fatalf("subscription = %+v, want plan_pro seq %d", after, seeded.PeriodSeq)
			}
		})
	}
}

type commitFailingRepo struct {
	*storefake.InMemorySubscriptionRepository
	failNext bool
}

var errCommitLost = errors.New("commit lost")

func (r *commitFailingRepo) WithSellerLock(ctx context.Context, sellerID string, fn func(ctx context.Context, tx store.SubscriptionTx) error) error {
	if !r.failNext {
		return r.InMemorySubscriptionRepository.WithSellerLock(ctx, sellerID, fn)
	}
	r.failNext = false
	return r.InMemorySubscriptionRepository.WithSellerLock(ctx, sellerID, func(ctx context.Context, tx store.SubscriptionTx) error {
		if err := fn(ctx, tx); err != nil {
			return err
		}
		return errCommitLost
	})
}

func TestSubscribeRetryReplaysChargeKey(t *testing.T) {
	// Scenario: Retry after a lost response is not charged twice
	repo := &commitFailingRepo{InMemorySubscriptionRepository: storefake.NewInMemorySubscriptionRepository()}
	pay := paymentfake.New()
	svc := New(repo, nil).WithPayment(pay)
	svc.nowFn = func() time.Time { return billingT0 }
	lapsed := seedRow{plan: "plan_pro", status: store.SubscriptionStatusExpired, start: billingT0.Add(-60 * day), end: billingT0.Add(-30 * day), seq: 1}
	seed(t, repo, "seller-1", lapsed)

	repo.failNext = true
	if _, err := svc.Subscribe(t.Context(), "seller-1", "plan_pro"); err == nil {
		t.Fatal("first Subscribe succeeded, want the lost commit to surface as an error")
	}
	if got := entitlementTier(t, svc, "seller-1"); got != store.PlanTierFree {
		t.Fatalf("tier after lost commit = %d, want FREE", got)
	}
	if len(pay.Fees()) != 1 {
		t.Fatalf("fees after first attempt = %d, want 1", len(pay.Fees()))
	}

	sub, err := svc.Subscribe(t.Context(), "seller-1", "plan_pro")
	if err != nil {
		t.Fatalf("retry Subscribe: %v", err)
	}
	calls := pay.Calls()
	if len(calls) != 2 {
		t.Fatalf("charge calls = %d, want 2 (original and replay)", len(calls))
	}
	if calls[0].ReferenceID != calls[1].ReferenceID {
		t.Fatalf("retry key %q differs from original %q", calls[1].ReferenceID, calls[0].ReferenceID)
	}
	if want := fmt.Sprintf("sub:%s:2:plan_pro", sub.ID); calls[1].ReferenceID != want {
		t.Fatalf("key = %q, want %q", calls[1].ReferenceID, want)
	}
	if fees := pay.Fees(); len(fees) != 1 || fees[0].Amount != 199000 {
		t.Fatalf("wallet fees = %+v, want exactly one of 199000", fees)
	}
	if sub.Status != store.SubscriptionStatusActive || sub.PlanID != "plan_pro" {
		t.Fatalf("subscription = %+v, want ACTIVE plan_pro", sub)
	}
}

func TestSubscribeUpgradeNoProration(t *testing.T) {
	// Scenario: Upgrade charges the full price without proration
	f := newBilling(t)
	start := f.now
	seed(t, f.repo, "seller-1", activeRow("plan_pro", start, start.Add(30*day)))
	f.now = start.Add(10 * day)

	sub, err := f.svc.Subscribe(t.Context(), "seller-1", "plan_premium")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	calls := f.pay.Calls()
	if len(calls) != 1 || calls[0].Amount != 499000 {
		t.Fatalf("charges = %+v, want one of 499000", calls)
	}
	if !sub.CurrentPeriodStart.Equal(f.now) || !sub.CurrentPeriodEnd.Equal(f.now.Add(30*day)) {
		t.Fatalf("period = %v..%v, want now..now+30d", sub.CurrentPeriodStart, sub.CurrentPeriodEnd)
	}
	if sub.PlanID != "plan_premium" || sub.PeriodSeq != 2 {
		t.Fatalf("subscription = %+v, want plan_premium seq 2", sub)
	}
	if want := fmt.Sprintf("sub:%s:2:plan_premium", sub.ID); calls[0].ReferenceID != want {
		t.Fatalf("key = %q, want %q", calls[0].ReferenceID, want)
	}
}

func TestSubscribeDowngradeDeferred(t *testing.T) {
	// Scenario: Downgrade is deferred
	f := newBilling(t)
	end := f.now.Add(12 * day)
	seed(t, f.repo, "seller-1", activeRow("plan_premium", end.Add(-30*day), end))

	sub, err := f.svc.Subscribe(t.Context(), "seller-1", "plan_pro")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if n := len(f.pay.Calls()); n != 0 {
		t.Fatalf("ChargeSellerFee calls = %d, want 0", n)
	}
	if got := entitlementTier(t, f.svc, "seller-1"); got != store.PlanTierPremium {
		t.Fatalf("tier = %d, want PREMIUM until the period ends", got)
	}
	mine, err := f.svc.GetMySubscription(t.Context(), "seller-1")
	if err != nil {
		t.Fatalf("GetMySubscription: %v", err)
	}
	if !mine.CurrentPeriodEnd.Equal(end) || !sub.CurrentPeriodEnd.Equal(end) {
		t.Fatalf("period end changed: response %v, stored %v, want %v", sub.CurrentPeriodEnd, mine.CurrentPeriodEnd, end)
	}
	if mine.PlanID != "plan_premium" || mine.PendingPlanID != "plan_pro" {
		t.Fatalf("stored = %+v, want plan_premium with pending plan_pro", mine)
	}
}

func TestCancelStopsAutoRenewKeepsPlan(t *testing.T) {
	// Scenario: Cancel keeps the plan until the end
	f := newBilling(t)
	end := f.now.Add(20 * day)
	seed(t, f.repo, "seller-1", activeRow("plan_pro", end.Add(-30*day), end))

	first, err := f.svc.Cancel(t.Context(), "seller-1")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if first.AutoRenew || !first.CurrentPeriodEnd.Equal(end) || first.Tier != store.PlanTierPro || first.Status != store.SubscriptionStatusActive {
		t.Fatalf("after cancel = %+v, want auto_renew false, same end, PRO, active", first)
	}
	second, err := f.svc.Cancel(t.Context(), "seller-1")
	if err != nil {
		t.Fatalf("second Cancel: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("second cancel changed state:\nfirst  %+v\nsecond %+v", first, second)
	}
	if got := entitlementTier(t, f.svc, "seller-1"); got != store.PlanTierPro {
		t.Fatalf("tier = %d, want PRO until the end", got)
	}
	if n := len(f.pay.Calls()); n != 0 {
		t.Fatalf("charge calls = %d, want 0", n)
	}
}

func TestCancelClearsPendingDowngrade(t *testing.T) {
	// Scenario: Cancel keeps the plan until the end
	f := newBilling(t)
	end := f.now.Add(12 * day)
	row := activeRow("plan_premium", end.Add(-30*day), end)
	row.pending = "plan_pro"
	seed(t, f.repo, "seller-1", row)

	got, err := f.svc.Cancel(t.Context(), "seller-1")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got.PendingPlanID != "" || got.AutoRenew || got.PlanID != "plan_premium" {
		t.Fatalf("after cancel = %+v, want no pending, auto_renew false, plan_premium", got)
	}
}

func TestCancelFreeSellerRejected(t *testing.T) {
	// Scenario: Cancel without a paid plan is rejected
	f := newBilling(t)
	if _, err := f.svc.Cancel(t.Context(), "seller-1"); !errors.Is(err, ErrNoPaidPlan) {
		t.Fatalf("Cancel(FREE) err = %v, want ErrNoPaidPlan", err)
	}
	if _, err := f.repo.GetActiveSubscriptionBySeller(t.Context(), "seller-1"); !errors.Is(err, store.ErrSubscriptionNotFound) {
		t.Fatalf("active row after rejected cancel = %v, want none", err)
	}

	lapsed := seedRow{plan: "plan_pro", status: store.SubscriptionStatusExpired, start: f.now.Add(-60 * day), end: f.now.Add(-30 * day), seq: 1}
	seed(t, f.repo, "seller-2", lapsed)
	before := stored(t, f.repo, "seller-2")
	if _, err := f.svc.Cancel(t.Context(), "seller-2"); !errors.Is(err, ErrNoPaidPlan) {
		t.Fatalf("Cancel(lapsed) err = %v, want ErrNoPaidPlan", err)
	}
	if after := stored(t, f.repo, "seller-2"); !reflect.DeepEqual(before, after) {
		t.Fatalf("lapsed row changed:\nbefore %+v\nafter  %+v", before, after)
	}
}

func TestSubscribeCurrentPlanResumes(t *testing.T) {
	// Scenario: Re-subscribing to the current plan resumes renewal
	f := newBilling(t)
	end := f.now.Add(15 * day)
	row := activeRow("plan_pro", end.Add(-30*day), end)
	row.autoRenew = false
	seed(t, f.repo, "seller-1", row)

	sub, err := f.svc.Subscribe(t.Context(), "seller-1", "plan_pro")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if n := len(f.pay.Calls()); n != 0 {
		t.Fatalf("ChargeSellerFee calls = %d, want 0", n)
	}
	if !sub.AutoRenew || !sub.CurrentPeriodEnd.Equal(end) || sub.PlanID != "plan_pro" {
		t.Fatalf("subscription = %+v, want auto_renew true, same end, plan_pro", sub)
	}
}

func TestSubscribeCurrentPlanRevertsPendingDowngrade(t *testing.T) {
	// Scenario: Re-subscribing to the current plan resumes renewal
	f := newBilling(t)
	end := f.now.Add(12 * day)
	row := activeRow("plan_premium", end.Add(-30*day), end)
	row.pending = "plan_pro"
	row.autoRenew = false
	seed(t, f.repo, "seller-1", row)

	sub, err := f.svc.Subscribe(t.Context(), "seller-1", "plan_premium")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if n := len(f.pay.Calls()); n != 0 {
		t.Fatalf("ChargeSellerFee calls = %d, want 0", n)
	}
	if sub.PendingPlanID != "" || !sub.AutoRenew || sub.PlanID != "plan_premium" {
		t.Fatalf("subscription = %+v, want no pending, auto_renew true, plan_premium", sub)
	}
}

func TestSubscribeUnknownPlanDoesNotCharge(t *testing.T) {
	// Scenario: Missing or unknown plan id is invalid
	f := newBilling(t)
	if _, err := f.svc.Subscribe(t.Context(), "seller-1", "plan_bogus"); !errors.Is(err, store.ErrPlanNotFound) {
		t.Fatalf("err = %v, want ErrPlanNotFound", err)
	}
	if _, err := f.svc.Subscribe(t.Context(), "seller-1", ""); !errors.Is(err, ErrInvalidSubscription) {
		t.Fatalf("err = %v, want ErrInvalidSubscription", err)
	}
	if n := len(f.pay.Calls()); n != 0 {
		t.Fatalf("ChargeSellerFee calls = %d, want 0", n)
	}
}
