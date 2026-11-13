package rpc_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-domain/generated/platform/common/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/rpc"
	"github.com/buidangphuc/team-domain/internal/store"
	"github.com/buidangphuc/team-domain/internal/store/storetest"
)

// newBundleHandler builds a ListingHandler wired with an in-memory bundle store
// (listing deps unused by the bundle RPCs).
func newBundleHandler() *rpc.ListingHandler {
	lrepo := storetest.NewInMemoryListingRepository(
		store.Listing{ID: "l1", SellerID: "seller_a", Price: 1_000_000_000},
		store.Listing{ID: "l2", SellerID: "seller_a", Price: 1_000_000_000},
		store.Listing{ID: "l3", SellerID: "seller_a", Price: 1_000_000_000},
		store.Listing{ID: "lb", SellerID: "seller_b", Price: 1_000_000_000},
		store.Listing{ID: "lb2", SellerID: "seller_b", Price: 1_000_000_000},
	)
	bSvc := listing.NewBundleService(storetest.NewInMemoryBundleRepository(), lrepo)
	return rpc.NewListingHandler(listing.NewListingService(lrepo), nil, nil).
		WithBundles(bSvc)
}

// TestCreateThenGetBundle: happy path — a seller creates a bundle and reads it
// back by the server-assigned id. Also asserts owner-scoping: the authenticated
// principal owns the row.
func TestCreateThenGetBundle(t *testing.T) {
	h := newBundleHandler()
	ctx := sellerCtx("seller_a")

	createRes, err := h.CreateBundle(ctx, &listingv1.CreateBundleRequest{
		Title:       "Combo nhà bếp",
		ListingIds:  []string{"l1", "l2", "l3"},
		BundlePrice: 2_500_000_000,
	})
	if err != nil {
		t.Fatalf("create bundle: %v", err)
	}
	created := createRes.GetBundle()
	if created.GetId() == "" {
		t.Fatalf("expected a server-assigned bundle id")
	}
	if created.GetSellerId() != "seller_a" {
		t.Errorf("owner-scope not enforced: got seller_id %q, want seller_a", created.GetSellerId())
	}
	if created.GetCreatedAt() == nil {
		t.Errorf("expected created_at to be set")
	}

	getRes, err := h.GetBundle(ctx, &listingv1.GetBundleRequest{Id: created.GetId()})
	if err != nil {
		t.Fatalf("get bundle: %v", err)
	}
	got := getRes.GetBundle()
	if got.GetTitle() != "Combo nhà bếp" || got.GetBundlePrice() != 2_500_000_000 {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if len(got.GetListingIds()) != 3 {
		t.Errorf("expected 3 listing ids, got %v", got.GetListingIds())
	}
}

// TestListBundlesBySeller: a seller's bundles are returned; another seller's
// bundles are excluded (cross-user isolation).
func TestListBundlesBySeller(t *testing.T) {
	h := newBundleHandler()

	for _, title := range []string{"Combo A1", "Combo A2"} {
		if _, err := h.CreateBundle(sellerCtx("seller_a"), &listingv1.CreateBundleRequest{Title: title, ListingIds: []string{"l1", "l2"}, BundlePrice: 1_500_000_000}); err != nil {
			t.Fatalf("seller_a create %q: %v", title, err)
		}
	}
	if _, err := h.CreateBundle(sellerCtx("seller_b"), &listingv1.CreateBundleRequest{Title: "Combo B1", ListingIds: []string{"lb", "lb2"}, BundlePrice: 1_500_000_000}); err != nil {
		t.Fatalf("seller_b create: %v", err)
	}

	res, err := h.ListBundlesBySeller(sellerCtx("seller_a"), &listingv1.ListBundlesBySellerRequest{
		SellerId: "seller_a",
	})
	if err != nil {
		t.Fatalf("list bundles: %v", err)
	}
	if len(res.GetBundles()) != 2 {
		t.Fatalf("expected 2 bundles for seller_a, got %d", len(res.GetBundles()))
	}
	for _, b := range res.GetBundles() {
		if b.GetSellerId() != "seller_a" {
			t.Errorf("cross-user leak: got a bundle owned by %q", b.GetSellerId())
		}
	}
	if res.GetPage().GetTotal() != 2 {
		t.Errorf("expected total 2, got %d", res.GetPage().GetTotal())
	}
}

// TestBundleEdgeCases: empty input and anonymous callers are handled without
// panics and with the right gRPC codes.
func TestBundleEdgeCases(t *testing.T) {
	h := newBundleHandler()
	ctx := sellerCtx("seller_a")

	t.Run("create missing title", func(t *testing.T) {
		_, err := h.CreateBundle(ctx, &listingv1.CreateBundleRequest{BundlePrice: 100})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("want InvalidArgument, got %v", err)
		}
	})

	t.Run("get empty id", func(t *testing.T) {
		_, err := h.GetBundle(ctx, &listingv1.GetBundleRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("want InvalidArgument, got %v", err)
		}
	})

	t.Run("get missing bundle", func(t *testing.T) {
		_, err := h.GetBundle(ctx, &listingv1.GetBundleRequest{Id: "nope"})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("want NotFound, got %v", err)
		}
	})

	t.Run("list empty seller_id", func(t *testing.T) {
		_, err := h.ListBundlesBySeller(ctx, &listingv1.ListBundlesBySellerRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("want InvalidArgument, got %v", err)
		}
	})

	t.Run("anonymous create denied", func(t *testing.T) {
		_, err := h.CreateBundle(context.Background(), &listingv1.CreateBundleRequest{Title: "x"})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("want Unauthenticated, got %v", err)
		}
	})

	t.Run("reader without write scope cannot create", func(t *testing.T) {
		readerCtx := contextWithPrincipal(context.Background(), &commonv1.Principal{
			Id:     "reader",
			Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
			Scopes: []string{"listing.read"},
		})
		_, err := h.CreateBundle(readerCtx, &listingv1.CreateBundleRequest{Title: "x"})
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("want PermissionDenied, got %v", err)
		}
	})
}

func newIntegrityHandler(seedBundles ...store.Bundle) *rpc.ListingHandler {
	seed := []store.Listing{
		{ID: "p1", SellerID: "seller_a", Price: 100000},
		{ID: "p2", SellerID: "seller_a", Price: 50000},
		{ID: "v1", SellerID: "seller_a", Price: 50000, Variants: []store.Variant{{ID: "var1"}}},
		{ID: "f1", SellerID: "seller_b", Price: 50000},
	}
	for i := 1; i <= 11; i++ {
		seed = append(seed, store.Listing{ID: fmt.Sprintf("m%d", i), SellerID: "seller_a", Price: 1000})
	}
	lrepo := storetest.NewInMemoryListingRepository(seed...)
	bSvc := listing.NewBundleService(storetest.NewInMemoryBundleRepository(seedBundles...), lrepo)
	return rpc.NewListingHandler(listing.NewListingService(lrepo), nil, nil).WithBundles(bSvc)
}

func mids(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("m%d", i+1)
	}
	return out
}

func listCount(t *testing.T, h *rpc.ListingHandler, seller string) int {
	t.Helper()
	res, err := h.ListBundlesBySeller(sellerCtx(seller), &listingv1.ListBundlesBySellerRequest{SellerId: seller})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return len(res.GetBundles())
}

func createErr(h *rpc.ListingHandler, ids []string, price int64) error {
	_, err := h.CreateBundle(sellerCtx("seller_a"), &listingv1.CreateBundleRequest{Title: "t", ListingIds: ids, BundlePrice: price})
	return err
}

// Scenario B2: another seller's listing is refused.
func TestB2_ForeignListingPermissionDenied(t *testing.T) {
	h := newIntegrityHandler()
	err := createErr(h, []string{"p1", "f1"}, 120000)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied, got %v", err)
	}
	if n := listCount(t, h, "seller_a"); n != 0 {
		t.Errorf("bundle stored: %d", n)
	}
}

// Scenario B3: unknown id gives the same code and message as B2.
func TestB3_UnknownListingSameAnswerAsForeign(t *testing.T) {
	h := newIntegrityHandler()
	foreign := status.Convert(createErr(h, []string{"p1", "f1"}, 120000))
	unknown := status.Convert(createErr(h, []string{"p1", "nope"}, 120000))
	if unknown.Code() != codes.PermissionDenied || foreign.Code() != codes.PermissionDenied {
		t.Fatalf("codes: foreign=%v unknown=%v, want PermissionDenied", foreign.Code(), unknown.Code())
	}
	if unknown.Message() != foreign.Message() || unknown.Message() == "" {
		t.Errorf("messages differ: %q vs %q", foreign.Message(), unknown.Message())
	}
	if strings.Contains(unknown.Message(), "nope") || strings.Contains(foreign.Message(), "f1") {
		t.Errorf("message leaks listing id: %q / %q", foreign.Message(), unknown.Message())
	}
}

// Scenario B4: caller without listing.write is refused and nothing is stored.
func TestB4_MissingWriteScopeStoresNothing(t *testing.T) {
	h := newIntegrityHandler()
	ctx := typedCtx("seller_a", "user", "listing.read")
	_, err := h.CreateBundle(ctx, &listingv1.CreateBundleRequest{Title: "t", ListingIds: []string{"p1", "p2"}, BundlePrice: 120000})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied, got %v", err)
	}
	if n := listCount(t, h, "seller_a"); n != 0 {
		t.Errorf("bundle stored: %d", n)
	}
}

// Scenarios B5, B6, B7: count and duplicate rules map to InvalidArgument.
func TestB5B6B7_CountAndDuplicateInvalidArgument(t *testing.T) {
	h := newIntegrityHandler()
	cases := []struct {
		name string
		ids  []string
	}{
		{"B5 single listing", []string{"p1"}},
		{"B6 eleven listings", mids(11)},
		{"B7 repeated id", []string{"p1", "p1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := createErr(h, c.ids, 500); status.Code(err) != codes.InvalidArgument {
				t.Fatalf("want InvalidArgument, got %v", err)
			}
		})
	}
	if n := listCount(t, h, "seller_a"); n != 0 {
		t.Errorf("bundle stored: %d", n)
	}
}

// Scenario B6: exactly ten listings is accepted.
func TestB6_TenListingsAccepted(t *testing.T) {
	h := newIntegrityHandler()
	res, err := h.CreateBundle(sellerCtx("seller_a"), &listingv1.CreateBundleRequest{Title: "t", ListingIds: mids(10), BundlePrice: 9999})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := len(res.GetBundle().GetListingIds()); got != 10 {
		t.Errorf("listing ids = %d, want 10", got)
	}
}

// Scenario B8: a variant listing maps to InvalidArgument.
func TestB8_VariantListingInvalidArgument(t *testing.T) {
	h := newIntegrityHandler()
	if err := createErr(h, []string{"p1", "v1"}, 120000); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument, got %v", err)
	}
	if n := listCount(t, h, "seller_a"); n != 0 {
		t.Errorf("bundle stored: %d", n)
	}
}

// Scenario B9: price equal to the sum maps to InvalidArgument.
func TestB9_PriceEqualToSumInvalidArgument(t *testing.T) {
	h := newIntegrityHandler()
	if err := createErr(h, []string{"p1", "p2"}, 150000); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument, got %v", err)
	}
	if n := listCount(t, h, "seller_a"); n != 0 {
		t.Errorf("bundle stored: %d", n)
	}
}

// Scenario B11: zero and negative prices map to InvalidArgument.
func TestB11_NonPositivePriceInvalidArgument(t *testing.T) {
	h := newIntegrityHandler()
	for _, price := range []int64{0, -1} {
		if err := createErr(h, []string{"p1", "p2"}, price); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("price %d: want InvalidArgument, got %v", price, err)
		}
	}
	if n := listCount(t, h, "seller_a"); n != 0 {
		t.Errorf("bundle stored: %d", n)
	}
}

// Scenario B12: a legacy bundle breaking the rules is still returned unchanged.
func TestB12_LegacyBundleStillReadable(t *testing.T) {
	legacy := store.Bundle{ID: "legacy1", SellerID: "seller_a", Title: "old", ListingIDs: []string{"p1", "p2"}, BundlePrice: 150000}
	h := newIntegrityHandler(legacy)
	ctx := sellerCtx("seller_a")

	got, err := h.GetBundle(ctx, &listingv1.GetBundleRequest{Id: "legacy1"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	b := got.GetBundle()
	if b.GetBundlePrice() != 150000 || len(b.GetListingIds()) != 2 || b.GetTitle() != "old" {
		t.Errorf("legacy bundle altered: %+v", b)
	}
	res, err := h.ListBundlesBySeller(ctx, &listingv1.ListBundlesBySellerRequest{SellerId: "seller_a"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(res.GetBundles()) != 1 || res.GetBundles()[0].GetId() != "legacy1" || res.GetBundles()[0].GetBundlePrice() != 150000 {
		t.Errorf("legacy bundle missing from list: %+v", res.GetBundles())
	}
}

// Scenario B13: unknown bundle id is NotFound.
func TestB13_UnknownBundleNotFound(t *testing.T) {
	h := newIntegrityHandler()
	_, err := h.GetBundle(sellerCtx("seller_a"), &listingv1.GetBundleRequest{Id: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("want NotFound, got %v", err)
	}
}
