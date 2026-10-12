package promotion

import (
	"context"
	"errors"
	"testing"

	"github.com/buidangphuc/team-promotion/internal/store"
)

func newSubscriptionSvc() *SubscriptionService {
	return NewSubscriptionService(store.NewInMemorySubscriptionRepository(), nil)
}

// TestListPlansReturnsSeededCatalog verifies the FREE/PRO/PREMIUM catalog is
// returned in tier order with limits attached.
func TestListPlansReturnsSeededCatalog(t *testing.T) {
	svc := newSubscriptionSvc()
	plans, err := svc.ListPlans(context.Background())
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

// TestSubscribeThenEntitlements covers the happy path: a seller subscribes to PRO
// and GetEntitlements then reports the PRO tier + limits.
func TestSubscribeThenEntitlements(t *testing.T) {
	svc := newSubscriptionSvc()
	ctx := context.Background()

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

// TestGetEntitlementsDefaultsToFreeForUnsubscribed covers the default path and
// cross-user isolation: seller-1 on PRO does not leak into seller-2's default FREE,
// and an empty seller id also resolves to FREE without panicking.
func TestGetEntitlementsDefaultsToFreeForUnsubscribed(t *testing.T) {
	svc := newSubscriptionSvc()
	ctx := context.Background()

	if _, err := svc.Subscribe(ctx, "seller-1", "plan_pro"); err != nil {
		t.Fatalf("Subscribe seller-1: %v", err)
	}

	// A different, unsubscribed seller must default to FREE (isolation).
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

	// Empty seller id (anonymous) also resolves to FREE, no panic.
	if tier, _, err := svc.GetEntitlements(ctx, ""); err != nil || tier != store.PlanTierFree {
		t.Fatalf("GetEntitlements(\"\") = (%d, %v), want FREE, nil", tier, err)
	}
}

// TestSubscribeUnknownPlanRejected covers the invalid-input edge: an unknown plan
// id is rejected and no subscription is created.
func TestSubscribeUnknownPlanRejected(t *testing.T) {
	svc := newSubscriptionSvc()
	ctx := context.Background()

	if _, err := svc.Subscribe(ctx, "seller-1", "plan_bogus"); !errors.Is(err, ErrPlanNotFound) {
		t.Fatalf("Subscribe(unknown) err = %v, want ErrPlanNotFound", err)
	}
	if _, err := svc.Subscribe(ctx, "", "plan_pro"); !errors.Is(err, ErrInvalidSubscription) {
		t.Fatalf("Subscribe(empty seller) err = %v, want ErrInvalidSubscription", err)
	}

	// After failed subscribes the seller is still on FREE.
	tier, _, err := svc.GetEntitlements(ctx, "seller-1")
	if err != nil || tier != store.PlanTierFree {
		t.Fatalf("tier after failed subscribe = (%d, %v), want FREE", tier, err)
	}
}

// TestReSubscribeSwitchesPlan verifies a seller taking a second plan switches tier
// rather than creating a duplicate subscription.
func TestReSubscribeSwitchesPlan(t *testing.T) {
	svc := newSubscriptionSvc()
	ctx := context.Background()

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
