package subscription

import (
	"errors"
	"testing"

	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
)

func newSubscriptionSvc() *Service {
	return New(storefake.NewInMemorySubscriptionRepository(), nil)
}

func TestListPlansReturnsSeededCatalog(t *testing.T) {
	svc := newSubscriptionSvc()
	plans, err := svc.ListPlans(t.Context())
	if err != nil {
		t.Fatalf("ListPlans: %v", err)
	}
	if len(plans) != 3 {
		t.Fatalf("want 3 plans, got %d", len(plans))
	}
	wantTiers := []int32{store.PlanTierFree, store.PlanTierPro, store.PlanTierPremium}
	for i, want := range wantTiers {
		if plans[i].Tier != want {
			t.Fatalf("plan[%d] tier = %d, want %d", i, plans[i].Tier, want)
		}
		if len(plans[i].Limits) == 0 {
			t.Fatalf("plan[%d] (%s) has no limits", i, plans[i].ID)
		}
	}
	if plans[0].Price != 0 {
		t.Fatalf("FREE price = %d, want 0", plans[0].Price)
	}
}

func TestSubscribeThenEntitlements(t *testing.T) {
	svc := newSubscriptionSvc()
	ctx := t.Context()

	sub, err := svc.Subscribe(ctx, "seller-1", "plan_pro")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if sub.SellerID != "seller-1" || sub.PlanID != "plan_pro" {
		t.Fatalf("unexpected subscription: %+v", sub)
	}
	if sub.Tier != store.PlanTierPro {
		t.Fatalf("tier = %d, want PRO(%d)", sub.Tier, store.PlanTierPro)
	}
	if sub.Status != store.SubscriptionStatusActive {
		t.Fatalf("status = %q, want active", sub.Status)
	}

	tier, limits, err := svc.GetEntitlements(ctx, "seller-1")
	if err != nil {
		t.Fatalf("GetEntitlements: %v", err)
	}
	if tier != store.PlanTierPro {
		t.Fatalf("entitlement tier = %d, want PRO(%d)", tier, store.PlanTierPro)
	}
	if limits["max_listings"] != "100" {
		t.Fatalf("PRO max_listings = %q, want 100", limits["max_listings"])
	}
}

func TestGetEntitlementsDefaultsToFreeForUnsubscribed(t *testing.T) {
	svc := newSubscriptionSvc()
	ctx := t.Context()

	if _, err := svc.Subscribe(ctx, "seller-1", "plan_pro"); err != nil {
		t.Fatalf("Subscribe seller-1: %v", err)
	}

	tier, limits, err := svc.GetEntitlements(ctx, "seller-2")
	if err != nil {
		t.Fatalf("GetEntitlements(seller-2): %v", err)
	}
	if tier != store.PlanTierFree {
		t.Fatalf("unsubscribed tier = %d, want FREE(%d)", tier, store.PlanTierFree)
	}
	if limits["max_listings"] != "10" {
		t.Fatalf("FREE max_listings = %q, want 10", limits["max_listings"])
	}

	if tier, _, err := svc.GetEntitlements(ctx, ""); err != nil || tier != store.PlanTierFree {
		t.Fatalf("GetEntitlements(\"\") = (%d, %v), want FREE, nil", tier, err)
	}
}

func TestSubscribeUnknownPlanRejected(t *testing.T) {
	svc := newSubscriptionSvc()
	ctx := t.Context()

	if _, err := svc.Subscribe(ctx, "seller-1", "plan_bogus"); !errors.Is(err, store.ErrPlanNotFound) {
		t.Fatalf("Subscribe(unknown) err = %v, want store.ErrPlanNotFound", err)
	}
	if _, err := svc.Subscribe(ctx, "", "plan_pro"); !errors.Is(err, ErrInvalidSubscription) {
		t.Fatalf("Subscribe(empty seller) err = %v, want ErrInvalidSubscription", err)
	}

	tier, _, err := svc.GetEntitlements(ctx, "seller-1")
	if err != nil || tier != store.PlanTierFree {
		t.Fatalf("tier after failed subscribe = (%d, %v), want FREE", tier, err)
	}
}

func TestReSubscribeSwitchesPlan(t *testing.T) {
	svc := newSubscriptionSvc()
	ctx := t.Context()

	if _, err := svc.Subscribe(ctx, "seller-1", "plan_pro"); err != nil {
		t.Fatalf("Subscribe PRO: %v", err)
	}
	if _, err := svc.Subscribe(ctx, "seller-1", "plan_premium"); err != nil {
		t.Fatalf("Subscribe PREMIUM: %v", err)
	}
	tier, limits, err := svc.GetEntitlements(ctx, "seller-1")
	if err != nil {
		t.Fatalf("GetEntitlements: %v", err)
	}
	if tier != store.PlanTierPremium {
		t.Fatalf("tier = %d, want PREMIUM(%d)", tier, store.PlanTierPremium)
	}
	if limits["priority_support"] != "true" {
		t.Fatalf("PREMIUM priority_support = %q, want true", limits["priority_support"])
	}
}
