package listing_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/store"
	"github.com/buidangphuc/team-domain/internal/store/storetest"
)

type sfEnt struct {
	ent     listing.Entitlements
	err     error
	calls   int
	sellers []string
}

func (f *sfEnt) Get(_ context.Context, sellerID string) (listing.Entitlements, error) {
	f.calls++
	f.sellers = append(f.sellers, sellerID)
	return f.ent, f.err
}

func sfIDs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("l-%d", i)
	}
	return out
}

func sfEnv(featured int64, seed ...store.Storefront) (*listing.StorefrontService, *storetest.InMemoryStorefrontRepository, *sfEnt) {
	repo := storetest.NewInMemoryStorefrontRepository(seed...)
	ent := &sfEnt{ent: listing.Entitlements{MaxListings: 10, FeaturedListings: featured}}
	return listing.NewStorefrontService(repo).WithEntitlements(ent), repo, ent
}

func TestStorefrontFeaturedAbovePlanCapRejected(t *testing.T) {
	prior := store.Storefront{SellerID: "s1", Slug: "old", FeaturedListingIDs: sfIDs(2)}
	svc, repo, _ := sfEnv(5, prior)
	_, err := svc.Upsert(context.Background(), store.Storefront{Slug: "new", FeaturedListingIDs: sfIDs(6)}, "s1")
	if !errors.Is(err, listing.ErrFeaturedLimit) {
		t.Fatalf("err = %v, want ErrFeaturedLimit", err)
	}
	got, _ := repo.GetBySeller(context.Background(), "s1")
	if got.Slug != "old" || len(got.FeaturedListingIDs) != 2 {
		t.Errorf("stored storefront changed: %+v", got)
	}
}

func TestStorefrontFeaturedAtPlanCapAccepted(t *testing.T) {
	svc, repo, _ := sfEnv(5)
	if _, err := svc.Upsert(context.Background(), store.Storefront{Slug: "a", FeaturedListingIDs: sfIDs(5)}, "s1"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, _ := repo.GetBySeller(context.Background(), "s1")
	if len(got.FeaturedListingIDs) != 5 {
		t.Errorf("stored %d ids, want 5", len(got.FeaturedListingIDs))
	}
}

func TestStorefrontFeaturedPlanCapOverridesFixedLimit(t *testing.T) {
	svc, repo, _ := sfEnv(50)
	if _, err := svc.Upsert(context.Background(), store.Storefront{Slug: "a", FeaturedListingIDs: sfIDs(9)}, "s1"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, _ := repo.GetBySeller(context.Background(), "s1")
	if len(got.FeaturedListingIDs) != 9 {
		t.Errorf("stored %d ids, want 9", len(got.FeaturedListingIDs))
	}
}

func TestStorefrontFreeSellerCannotFeature(t *testing.T) {
	svc, repo, _ := sfEnv(0)
	_, err := svc.Upsert(context.Background(), store.Storefront{Slug: "a", FeaturedListingIDs: sfIDs(1)}, "s1")
	if !errors.Is(err, listing.ErrFeaturedLimit) {
		t.Fatalf("err = %v, want ErrFeaturedLimit", err)
	}
	if _, err := repo.GetBySeller(context.Background(), "s1"); !errors.Is(err, store.ErrStorefrontNotFound) {
		t.Errorf("rejected write persisted: %v", err)
	}
	if _, err := svc.Upsert(context.Background(), store.Storefront{Slug: "a"}, "s1"); err != nil {
		t.Errorf("upsert with 0 featured ids: %v", err)
	}
}

func TestStorefrontUnlimitedPlanNeverBlocked(t *testing.T) {
	svc, _, _ := sfEnv(-1)
	if _, err := svc.Upsert(context.Background(), store.Storefront{Slug: "a", FeaturedListingIDs: sfIDs(200)}, "s1"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
}

func TestStorefrontCapUsesPrincipalNotBodySeller(t *testing.T) {
	a := store.Storefront{SellerID: "A", Slug: "a-shop", Tagline: "keep"}
	svc, repo, ent := sfEnv(50, a)
	if _, err := svc.Upsert(context.Background(), store.Storefront{SellerID: "A", Slug: "b-shop", FeaturedListingIDs: sfIDs(3)}, "B"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if len(ent.sellers) != 1 || ent.sellers[0] != "B" {
		t.Errorf("entitlement looked up for %v, want [B]", ent.sellers)
	}
	gotB, err := repo.GetBySeller(context.Background(), "B")
	if err != nil || len(gotB.FeaturedListingIDs) != 3 {
		t.Errorf("B storefront = %+v, %v", gotB, err)
	}
	gotA, _ := repo.GetBySeller(context.Background(), "A")
	if gotA.Slug != "a-shop" || gotA.Tagline != "keep" || len(gotA.FeaturedListingIDs) != 0 {
		t.Errorf("A storefront changed: %+v", gotA)
	}
}

func TestStorefrontFeaturedFailsClosed(t *testing.T) {
	prior := store.Storefront{SellerID: "s1", Slug: "old"}
	svc, repo, ent := sfEnv(5, prior)
	ent.err = fmt.Errorf("%w: down", listing.ErrEntitlementsUnavailable)
	_, err := svc.Upsert(context.Background(), store.Storefront{Slug: "new", FeaturedListingIDs: sfIDs(1)}, "s1")
	if !errors.Is(err, listing.ErrEntitlementsUnavailable) {
		t.Fatalf("err = %v, want ErrEntitlementsUnavailable", err)
	}
	got, _ := repo.GetBySeller(context.Background(), "s1")
	if got.Slug != "old" {
		t.Errorf("stored storefront changed: %+v", got)
	}
}

func TestStorefrontWithoutFeaturedIgnoresEntitlements(t *testing.T) {
	svc, repo, ent := sfEnv(5)
	ent.err = fmt.Errorf("%w: down", listing.ErrEntitlementsUnavailable)
	if _, err := svc.Upsert(context.Background(), store.Storefront{Slug: "a"}, "s1"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if ent.calls != 0 {
		t.Errorf("entitlements called %d times, want 0", ent.calls)
	}
	if _, err := repo.GetBySeller(context.Background(), "s1"); err != nil {
		t.Errorf("not stored: %v", err)
	}
}
