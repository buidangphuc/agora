package store_test

import (
	"context"
	"testing"

	"github.com/buidangphuc/team-domain/internal/store"
	"github.com/buidangphuc/team-domain/internal/store/storetest"
)

type byIDsReader interface {
	Create(ctx context.Context, l store.Listing) (store.Listing, error)
	GetByIDs(ctx context.Context, ids []string) (map[string]store.Listing, error)
}

func byIDsRepos(t *testing.T) map[string]byIDsReader {
	t.Helper()
	pg, _ := newPGRepo(t.Context(), t)
	return map[string]byIDsReader{
		"postgres": pg,
		"memory":   storetest.NewInMemoryListingRepository(store.Listing{ID: "unrelated", Status: "published"}),
	}
}

func seedByIDsListing(t *testing.T, r byIDsReader, id, seller string, price int64, variants ...store.Variant) {
	t.Helper()
	if _, err := r.Create(t.Context(), store.Listing{
		ID: id, Title: id, Currency: "VND", Status: "published",
		SellerID: seller, Price: price, Stock: 5, Variants: variants,
	}); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func TestGetByIDs_B1_B10_ReturnsOwnerAndPriceForEveryId(t *testing.T) {
	for name, r := range byIDsRepos(t) {
		t.Run(name, func(t *testing.T) {
			a, b := newID(), newID()
			seedByIDsListing(t, r, a, "seller-1", 100000)
			seedByIDsListing(t, r, b, "seller-1", 50000)
			got, err := r.GetByIDs(t.Context(), []string{b, a})
			if err != nil {
				t.Fatalf("GetByIDs: %v", err)
			}
			if len(got) != 2 {
				t.Fatalf("want 2 listings, got %d", len(got))
			}
			if got[a].SellerID != "seller-1" || got[a].Price != 100000 {
				t.Errorf("a: seller=%q price=%d", got[a].SellerID, got[a].Price)
			}
			if got[b].SellerID != "seller-1" || got[b].Price != 50000 {
				t.Errorf("b: seller=%q price=%d", got[b].SellerID, got[b].Price)
			}
		})
	}
}

func TestGetByIDs_B2_ForeignOwnerIsVisibleToCaller(t *testing.T) {
	for name, r := range byIDsRepos(t) {
		t.Run(name, func(t *testing.T) {
			own, foreign := newID(), newID()
			seedByIDsListing(t, r, own, "seller-1", 100)
			seedByIDsListing(t, r, foreign, "seller-2", 200)
			got, err := r.GetByIDs(t.Context(), []string{own, foreign})
			if err != nil {
				t.Fatalf("GetByIDs: %v", err)
			}
			if got[own].SellerID != "seller-1" || got[foreign].SellerID != "seller-2" {
				t.Errorf("owners not reported: own=%q foreign=%q", got[own].SellerID, got[foreign].SellerID)
			}
		})
	}
}

func TestGetByIDs_B3_UnknownIdIsOmittedWithoutError(t *testing.T) {
	for name, r := range byIDsRepos(t) {
		t.Run(name, func(t *testing.T) {
			own, missing := newID(), newID()
			seedByIDsListing(t, r, own, "seller-1", 100)
			got, err := r.GetByIDs(t.Context(), []string{own, missing})
			if err != nil {
				t.Fatalf("unknown id must not error: %v", err)
			}
			if _, ok := got[own]; !ok {
				t.Errorf("own listing missing from result")
			}
			if _, ok := got[missing]; ok {
				t.Errorf("unknown id must be absent from result")
			}
			if len(got) != 1 {
				t.Errorf("want 1 listing, got %d", len(got))
			}
		})
	}
}

func TestGetByIDs_B8_VariantsAreLoaded(t *testing.T) {
	for name, r := range byIDsRepos(t) {
		t.Run(name, func(t *testing.T) {
			plain, withV := newID(), newID()
			seedByIDsListing(t, r, plain, "seller-1", 100)
			seedByIDsListing(t, r, withV, "seller-1", 200,
				store.Variant{Name: "S", SKU: "s", Price: 200, Stock: 1},
				store.Variant{Name: "M", SKU: "m", Price: 210, Stock: 2})
			got, err := r.GetByIDs(t.Context(), []string{plain, withV})
			if err != nil {
				t.Fatalf("GetByIDs: %v", err)
			}
			if n := len(got[withV].Variants); n != 2 {
				t.Errorf("variant listing: want 2 variants, got %d", n)
			}
			if n := len(got[plain].Variants); n != 0 {
				t.Errorf("plain listing: want 0 variants, got %d", n)
			}
		})
	}
}

func TestGetByIDs_B9_B10_PricesAreExactForSumCheck(t *testing.T) {
	for name, r := range byIDsRepos(t) {
		t.Run(name, func(t *testing.T) {
			a, b := newID(), newID()
			seedByIDsListing(t, r, a, "seller-1", 100000)
			seedByIDsListing(t, r, b, "seller-1", 50000)
			got, err := r.GetByIDs(t.Context(), []string{a, b})
			if err != nil {
				t.Fatalf("GetByIDs: %v", err)
			}
			if sum := got[a].Price + got[b].Price; sum != 150000 {
				t.Errorf("price sum = %d, want 150000", sum)
			}
		})
	}
}

func TestGetByIDs_B1_EmptyAndDuplicateInput(t *testing.T) {
	for name, r := range byIDsRepos(t) {
		t.Run(name, func(t *testing.T) {
			a := newID()
			seedByIDsListing(t, r, a, "seller-1", 100)
			got, err := r.GetByIDs(t.Context(), nil)
			if err != nil || len(got) != 0 {
				t.Errorf("empty input: got %d, err %v", len(got), err)
			}
			got, err = r.GetByIDs(t.Context(), []string{a, a})
			if err != nil {
				t.Fatalf("duplicate input: %v", err)
			}
			if len(got) != 1 || got[a].ID != a {
				t.Errorf("duplicate input: want exactly listing a, got %d entries", len(got))
			}
		})
	}
}
