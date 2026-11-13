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

const (
	sellerA = "seller_a"
	sellerB = "seller_b"
)

func bundleFixture(extra ...store.Listing) (*listing.BundleService, *storetest.InMemoryBundleRepository) {
	seed := []store.Listing{
		{ID: "a1", SellerID: sellerA, Price: 100000, Status: "published"},
		{ID: "a2", SellerID: sellerA, Price: 50000, Status: "published"},
		{ID: "b1", SellerID: sellerB, Price: 70000, Status: "published"},
		{ID: "av", SellerID: sellerA, Price: 80000, Status: "published",
			Variants: []store.Variant{{ID: "v1", ListingID: "av", Price: 80000}}},
	}
	seed = append(seed, extra...)
	bundles := storetest.NewInMemoryBundleRepository()
	return listing.NewBundleService(bundles, storetest.NewInMemoryListingRepository(seed...)), bundles
}

func mkBundle(price int64, ids ...string) store.Bundle {
	return store.Bundle{Title: "combo", ListingIDs: ids, BundlePrice: price}
}

func storedCount(t *testing.T, r *storetest.InMemoryBundleRepository, seller string) int {
	t.Helper()
	page, err := r.ListBySeller(context.Background(), seller, "", 100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return len(page.Items)
}

func TestBundleCreate_B1_OwnListingsStoredInRequestOrder(t *testing.T) {
	svc, repo := bundleFixture()
	got, err := svc.Create(context.Background(), mkBundle(149999, "a2", "a1"), sellerA)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.SellerID != sellerA || len(got.ListingIDs) != 2 || got.ListingIDs[0] != "a2" || got.ListingIDs[1] != "a1" {
		t.Fatalf("unexpected bundle %+v", got)
	}
	if _, err := repo.Get(context.Background(), got.ID); err != nil {
		t.Fatalf("not stored: %v", err)
	}
}

func TestBundleCreate_B2_ForeignListingRefused(t *testing.T) {
	svc, repo := bundleFixture()
	_, err := svc.Create(context.Background(), mkBundle(100000, "a1", "b1"), sellerA)
	if !errors.Is(err, listing.ErrBundleListingNotOwned) {
		t.Fatalf("want ErrBundleListingNotOwned, got %v", err)
	}
	if n := storedCount(t, repo, sellerA); n != 0 {
		t.Fatalf("stored %d bundles", n)
	}
}

func TestBundleCreate_B3_UnknownListingSameAnswerAsForeign(t *testing.T) {
	svc, repo := bundleFixture()
	_, foreign := svc.Create(context.Background(), mkBundle(100000, "a1", "b1"), sellerA)
	_, unknown := svc.Create(context.Background(), mkBundle(100000, "a1", "ghost"), sellerA)
	if !errors.Is(unknown, listing.ErrBundleListingNotOwned) {
		t.Fatalf("want ErrBundleListingNotOwned, got %v", unknown)
	}
	if foreign == nil || unknown.Error() != foreign.Error() {
		t.Fatalf("messages differ: foreign=%v unknown=%v", foreign, unknown)
	}
	if n := storedCount(t, repo, sellerA); n != 0 {
		t.Fatalf("stored %d bundles", n)
	}
}

func TestBundleCreate_B5_SingleListingRejected(t *testing.T) {
	svc, repo := bundleFixture()
	for name, ids := range map[string][]string{"one": {"a1"}, "none": nil} {
		_, err := svc.Create(context.Background(), mkBundle(1, ids...), sellerA)
		if !errors.Is(err, listing.ErrBundleListingCount) {
			t.Fatalf("%s: want ErrBundleListingCount, got %v", name, err)
		}
	}
	if n := storedCount(t, repo, sellerA); n != 0 {
		t.Fatalf("stored %d bundles", n)
	}
}

func TestBundleCreate_B6_TenAcceptedElevenRejected(t *testing.T) {
	var extra []store.Listing
	var ids []string
	for i := 0; i < 11; i++ {
		id := fmt.Sprintf("m%02d", i)
		extra = append(extra, store.Listing{ID: id, SellerID: sellerA, Price: 1000, Status: "published"})
		ids = append(ids, id)
	}
	svc, repo := bundleFixture(extra...)
	got, err := svc.Create(context.Background(), mkBundle(9999, ids[:10]...), sellerA)
	if err != nil {
		t.Fatalf("ten: %v", err)
	}
	if len(got.ListingIDs) != 10 {
		t.Fatalf("want 10 ids, got %d", len(got.ListingIDs))
	}
	_, err = svc.Create(context.Background(), mkBundle(10999, ids...), sellerA)
	if !errors.Is(err, listing.ErrBundleListingCount) {
		t.Fatalf("eleven: want ErrBundleListingCount, got %v", err)
	}
	if n := storedCount(t, repo, sellerA); n != 1 {
		t.Fatalf("want 1 stored, got %d", n)
	}
}

func TestBundleCreate_B7_RepeatedListingRejected(t *testing.T) {
	svc, repo := bundleFixture()
	_, err := svc.Create(context.Background(), mkBundle(100000, "a1", "a1"), sellerA)
	if !errors.Is(err, listing.ErrBundleDuplicateListing) {
		t.Fatalf("want ErrBundleDuplicateListing, got %v", err)
	}
	if n := storedCount(t, repo, sellerA); n != 0 {
		t.Fatalf("stored %d bundles", n)
	}
}

func TestBundleCreate_B8_VariantListingRejected(t *testing.T) {
	svc, repo := bundleFixture()
	_, err := svc.Create(context.Background(), mkBundle(100000, "a1", "av"), sellerA)
	if !errors.Is(err, listing.ErrBundleVariantListing) {
		t.Fatalf("want ErrBundleVariantListing, got %v", err)
	}
	if n := storedCount(t, repo, sellerA); n != 0 {
		t.Fatalf("stored %d bundles", n)
	}
}

func TestBundleCreate_B9_PriceEqualToSumRejected(t *testing.T) {
	svc, repo := bundleFixture()
	for _, price := range []int64{150000, 150001} {
		_, err := svc.Create(context.Background(), mkBundle(price, "a1", "a2"), sellerA)
		if !errors.Is(err, listing.ErrBundlePrice) {
			t.Fatalf("price %d: want ErrBundlePrice, got %v", price, err)
		}
	}
	if n := storedCount(t, repo, sellerA); n != 0 {
		t.Fatalf("stored %d bundles", n)
	}
}

func TestBundleCreate_B10_OneMinorUnitBelowSumAccepted(t *testing.T) {
	svc, _ := bundleFixture()
	got, err := svc.Create(context.Background(), mkBundle(149999, "a1", "a2"), sellerA)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.BundlePrice != 149999 {
		t.Fatalf("price %d", got.BundlePrice)
	}
}

func TestBundleCreate_B11_ZeroOrNegativePriceRejected(t *testing.T) {
	svc, repo := bundleFixture()
	for _, price := range []int64{0, -1} {
		_, err := svc.Create(context.Background(), mkBundle(price, "a1", "a2"), sellerA)
		if !errors.Is(err, listing.ErrBundlePrice) {
			t.Fatalf("price %d: want ErrBundlePrice, got %v", price, err)
		}
	}
	if n := storedCount(t, repo, sellerA); n != 0 {
		t.Fatalf("stored %d bundles", n)
	}
}
