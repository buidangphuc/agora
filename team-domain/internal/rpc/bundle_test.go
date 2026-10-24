package handler_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-domain/generated/platform/common/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/handler"
	"github.com/buidangphuc/team-domain/internal/repository"
	"github.com/buidangphuc/team-domain/internal/service"
)

// newBundleHandler builds a ListingHandler wired with an in-memory bundle store
// (listing deps unused by the bundle RPCs).
func newBundleHandler() *handler.ListingHandler {
	bSvc := service.NewBundleService(repository.NewInMemoryBundleRepository())
	return handler.NewListingHandler(service.NewListingService(repository.NewInMemoryListingRepository()), nil, nil).
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
		if _, err := h.CreateBundle(sellerCtx("seller_a"), &listingv1.CreateBundleRequest{Title: title}); err != nil {
			t.Fatalf("seller_a create %q: %v", title, err)
		}
	}
	if _, err := h.CreateBundle(sellerCtx("seller_b"), &listingv1.CreateBundleRequest{Title: "Combo B1"}); err != nil {
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
