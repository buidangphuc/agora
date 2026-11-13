package rpc_test

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
)

func TestSubscribeChargesThroughRPC(t *testing.T) {
	// Scenario: FREE seller subscribes to PRO
	r := startRig(t, rigOpts{})
	resp, err := r.sub.Subscribe(sellerCtx(t), &promotionv1.SubscribeRequest{PlanId: "plan_pro"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	calls := r.pay.Calls()
	if len(calls) != 1 || calls[0].SellerID != "seller-1" || calls[0].Amount != 199000 {
		t.Fatalf("charges = %+v, want one of 199000 for the principal seller-1", calls)
	}
	s := resp.GetSubscription()
	if s.GetPlanId() != "plan_pro" || s.GetStatus() != promotionv1.SubscriptionStatus_SUBSCRIPTION_STATUS_ACTIVE || !s.GetAutoRenew() {
		t.Fatalf("subscription = %+v, want ACTIVE plan_pro auto_renew", s)
	}
	end := s.GetCurrentPeriodEnd().AsTime()
	if d := time.Until(end); d < 29*24*time.Hour || d > 31*24*time.Hour {
		t.Fatalf("current_period_end in %v, want about 30 days", d)
	}
}

func TestSubscribeMapsPaymentFailures(t *testing.T) {
	// Scenario: Insufficient withdrawable balance is rejected
	// Scenario: Payment service unavailable fails without state change
	for name, tc := range map[string]struct {
		payErr error
		want   codes.Code
	}{
		"insufficient balance": {status.Error(codes.FailedPrecondition, "insufficient"), codes.FailedPrecondition},
		"payment down":         {status.Error(codes.Unavailable, "down"), codes.Unavailable},
		"payment timeout":      {status.Error(codes.DeadlineExceeded, "2s"), codes.Unavailable},
	} {
		t.Run(name, func(t *testing.T) {
			r := startRig(t, rigOpts{})
			r.pay.SetError(tc.payErr)
			if _, err := r.sub.Subscribe(sellerCtx(t), &promotionv1.SubscribeRequest{PlanId: "plan_pro"}); code(err) != tc.want {
				t.Fatalf("Subscribe code = %v, want %v (err=%v)", code(err), tc.want, err)
			}
			got, err := r.sub.GetMySubscription(sellerCtx(t), &promotionv1.GetMySubscriptionRequest{})
			if err != nil || got.GetSubscription() != nil {
				t.Fatalf("GetMySubscription = %+v, %v, want empty after a failed charge", got, err)
			}
		})
	}
}

func TestSubscribeRejectsBadPlanWithoutCharging(t *testing.T) {
	// Scenario: Missing or unknown plan id is invalid
	r := startRig(t, rigOpts{})
	if _, err := r.sub.Subscribe(sellerCtx(t), &promotionv1.SubscribeRequest{}); code(err) != codes.InvalidArgument {
		t.Fatalf("empty plan_id code = %v, want InvalidArgument (err=%v)", code(err), err)
	}
	if _, err := r.sub.Subscribe(sellerCtx(t), &promotionv1.SubscribeRequest{PlanId: "plan_bogus"}); code(err) != codes.NotFound {
		t.Fatalf("unknown plan_id code = %v, want NotFound (err=%v)", code(err), err)
	}
	if n := len(r.pay.Calls()); n != 0 {
		t.Fatalf("ChargeSellerFee calls = %d, want 0", n)
	}
}

func TestSubscribeRequiresSellerScope(t *testing.T) {
	// Scenario: Subscribe without seller scope is denied
	r := startRig(t, rigOpts{})
	for name, ctx := range map[string]context.Context{"buyer": buyerCtx(t), "admin only": authCtx(t, "admin-1", "admin")} {
		if _, err := r.sub.Subscribe(ctx, &promotionv1.SubscribeRequest{PlanId: "plan_pro"}); code(err) != codes.PermissionDenied {
			t.Errorf("%s: code = %v, want PermissionDenied (err=%v)", name, code(err), err)
		}
	}
	if n := len(r.pay.Calls()); n != 0 {
		t.Fatalf("ChargeSellerFee calls = %d, want 0", n)
	}
}

func TestCancelSubscriptionRequiresSellerScope(t *testing.T) {
	// Scenario: Cancel without seller scope is denied
	r := startRig(t, rigOpts{})
	if _, err := r.sub.Subscribe(sellerCtx(t), &promotionv1.SubscribeRequest{PlanId: "plan_pro"}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if _, err := r.sub.CancelSubscription(buyerCtx(t), &promotionv1.CancelSubscriptionRequest{}); code(err) != codes.PermissionDenied {
		t.Fatalf("buyer cancel code = %v, want PermissionDenied (err=%v)", code(err), err)
	}
	got, err := r.sub.GetMySubscription(sellerCtx(t), &promotionv1.GetMySubscriptionRequest{})
	if err != nil || !got.GetSubscription().GetAutoRenew() {
		t.Fatalf("subscription after denied cancel = %+v, %v, want auto_renew still true", got, err)
	}
}

func TestCancelSubscriptionKeepsPlanThroughRPC(t *testing.T) {
	// Scenario: Cancel keeps the plan until the end
	r := startRig(t, rigOpts{})
	sub, err := r.sub.Subscribe(sellerCtx(t), &promotionv1.SubscribeRequest{PlanId: "plan_pro"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	resp, err := r.sub.CancelSubscription(sellerCtx(t), &promotionv1.CancelSubscriptionRequest{})
	if err != nil {
		t.Fatalf("CancelSubscription: %v", err)
	}
	s := resp.GetSubscription()
	if s.GetAutoRenew() || s.GetTier() != promotionv1.PlanTier_PLAN_TIER_PRO ||
		!s.GetCurrentPeriodEnd().AsTime().Equal(sub.GetSubscription().GetCurrentPeriodEnd().AsTime()) {
		t.Fatalf("after cancel = %+v, want auto_renew false, PRO, same period end", s)
	}
}

func TestCancelSubscriptionFreeSellerFailsPrecondition(t *testing.T) {
	// Scenario: Cancel without a paid plan is rejected
	r := startRig(t, rigOpts{})
	if _, err := r.sub.CancelSubscription(sellerCtx(t), &promotionv1.CancelSubscriptionRequest{}); code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %v, want FailedPrecondition (err=%v)", code(err), err)
	}
}

func TestGetMySubscriptionReturnsOwn(t *testing.T) {
	// Scenario: Paid seller reads own subscription
	r := startRig(t, rigOpts{})
	if _, err := r.sub.Subscribe(sellerCtx(t), &promotionv1.SubscribeRequest{PlanId: "plan_pro"}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	got, err := r.sub.GetMySubscription(sellerCtx(t), &promotionv1.GetMySubscriptionRequest{})
	if err != nil {
		t.Fatalf("GetMySubscription: %v", err)
	}
	s := got.GetSubscription()
	if s.GetPlanId() != "plan_pro" || s.GetSellerId() != "seller-1" ||
		s.GetStatus() != promotionv1.SubscriptionStatus_SUBSCRIPTION_STATUS_ACTIVE || !s.GetAutoRenew() ||
		s.GetCurrentPeriodEnd() == nil || !s.GetCurrentPeriodEnd().IsValid() {
		t.Fatalf("subscription = %+v, want own ACTIVE plan_pro with period end and auto_renew", s)
	}
}

func TestGetMySubscriptionFreeIsEmpty(t *testing.T) {
	// Scenario: FREE seller gets an empty response
	r := startRig(t, rigOpts{})
	got, err := r.sub.GetMySubscription(sellerCtx(t), &promotionv1.GetMySubscriptionRequest{})
	if err != nil {
		t.Fatalf("GetMySubscription: %v, want OK", err)
	}
	if got.GetSubscription() != nil {
		t.Fatalf("subscription = %+v, want none", got.GetSubscription())
	}
}

func TestSubscriptionRPCsIgnoreOtherSellers(t *testing.T) {
	// Scenario: Another seller's subscription is not reachable
	r := startRig(t, rigOpts{})
	if _, err := r.sub.Subscribe(sellerCtx(t), &promotionv1.SubscribeRequest{PlanId: "plan_pro"}); err != nil {
		t.Fatalf("Subscribe seller-1: %v", err)
	}
	other := authCtx(t, "seller-2", "listing.write")

	got, err := r.sub.GetMySubscription(other, &promotionv1.GetMySubscriptionRequest{})
	if err != nil || got.GetSubscription() != nil {
		t.Fatalf("seller-2 GetMySubscription = %+v, %v, want empty", got, err)
	}
	if _, err := r.sub.CancelSubscription(other, &promotionv1.CancelSubscriptionRequest{}); code(err) != codes.FailedPrecondition {
		t.Fatalf("seller-2 cancel code = %v, want FailedPrecondition (no own paid plan)", code(err))
	}
	mine, err := r.sub.GetMySubscription(sellerCtx(t), &promotionv1.GetMySubscriptionRequest{})
	if err != nil || !mine.GetSubscription().GetAutoRenew() || mine.GetSubscription().GetPlanId() != "plan_pro" {
		t.Fatalf("seller-1 subscription = %+v, %v, want PRO with auto_renew true", mine, err)
	}
}
