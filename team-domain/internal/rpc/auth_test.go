package rpc_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/rpc"
	"github.com/buidangphuc/team-domain/internal/store"
	"github.com/buidangphuc/team-domain/internal/store/storetest"
)

func reservationFixture() (*rpc.ListingHandler, *storetest.InMemoryListingRepository) {
	repo := storetest.NewInMemoryListingRepository(store.Listing{ID: "L-1", Title: "Phone", Currency: "VND", Status: "published", SellerID: "seller-a", Stock: 10})
	return rpc.NewListingHandler(listing.NewListingService(repo), nil, nil), repo
}

func reservationCalls(h *rpc.ListingHandler) map[string]func(context.Context) error {
	return map[string]func(context.Context) error{
		"ReserveStock": func(ctx context.Context) error {
			_, err := h.ReserveStock(ctx, &listingv1.ReserveStockRequest{ListingId: "L-1", Quantity: 1, ReservationId: "r-1"})
			return err
		},
		"ReleaseStock": func(ctx context.Context) error {
			_, err := h.ReleaseStock(ctx, &listingv1.ReleaseStockRequest{ListingId: "L-1", Quantity: 1, ReservationId: "r-1"})
			return err
		},
		"CommitReservation": func(ctx context.Context) error {
			_, err := h.CommitReservation(ctx, &listingv1.CommitReservationRequest{ReservationId: "r-1"})
			return err
		},
	}
}

// Scenario: Domain SDK: A call without a principal is Unauthenticated.
func TestRPCsWithoutPrincipalAreUnauthenticated(t *testing.T) {
	h, repo := reservationFixture()
	mh := newMHFixture().h
	calls := map[string]func(context.Context) error{
		"CreateListing": func(ctx context.Context) error {
			_, err := h.CreateListing(ctx, &listingv1.CreateListingRequest{Listing: &listingv1.Listing{Title: "x"}})
			return err
		},
		"UpdateListing": func(ctx context.Context) error {
			_, err := h.UpdateListing(ctx, &listingv1.UpdateListingRequest{Listing: &listingv1.Listing{Id: "L-1"}})
			return err
		},
		"DeleteListing": func(ctx context.Context) error {
			_, err := h.DeleteListing(ctx, &listingv1.DeleteListingRequest{Id: "L-1"})
			return err
		},
		"ListMyListings": func(ctx context.Context) error {
			_, err := h.ListMyListings(ctx, &listingv1.ListMyListingsRequest{})
			return err
		},
		"ModerateListing": func(ctx context.Context) error {
			_, err := mh.ModerateListing(ctx, modReq("L-1", decApprove, "", ""))
			return err
		},
		"ListModerationQueue": func(ctx context.Context) error {
			_, err := mh.ListModerationQueue(ctx, &listingv1.ListModerationQueueRequest{})
			return err
		},
		"UpsertStorefront": func(ctx context.Context) error {
			_, err := newStorefrontHandler().UpsertStorefront(ctx, &listingv1.UpsertStorefrontRequest{Storefront: &listingv1.Storefront{Slug: "s"}})
			return err
		},
		"CreateBundle": func(ctx context.Context) error {
			_, err := newBundleHandler().CreateBundle(ctx, &listingv1.CreateBundleRequest{Title: "x"})
			return err
		},
	}
	for name, call := range reservationCalls(h) {
		calls[name] = call
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			err := call(t.Context())
			st := status.Convert(err)
			if st.Code() != codes.Unauthenticated || st.Message() != "authentication required" {
				t.Fatalf("got %v %q, want Unauthenticated %q", st.Code(), st.Message(), "authentication required")
			}
		})
	}
	if got, _ := repo.Get(t.Context(), "L-1"); got.Stock != 10 {
		t.Fatalf("stock changed to %d by unauthenticated calls", got.Stock)
	}
}

// Scenario: Domain SDK: A user principal cannot call a reservation RPC.
func TestUserPrincipalCannotCallReservationRPCs(t *testing.T) {
	principals := map[string]context.Context{
		"reader user":            typedCtx("u-1", "user", "listing.read"),
		"seller user with write": typedCtx("seller-a", "user", "listing.read", "listing.write"),
		"admin user":             typedCtx("admin-1", "user", "listing.read", "listing.write", "admin"),
	}
	for pname, ctx := range principals {
		h, repo := reservationFixture()
		for cname, call := range reservationCalls(h) {
			t.Run(pname+"/"+cname, func(t *testing.T) {
				st := status.Convert(call(ctx))
				if st.Code() != codes.PermissionDenied || st.Message() != "principal type not allowed" {
					t.Fatalf("got %v %q, want PermissionDenied %q", st.Code(), st.Message(), "principal type not allowed")
				}
			})
		}
		if got, _ := repo.Get(t.Context(), "L-1"); got.Stock != 10 {
			t.Fatalf("%s: stock changed to %d", pname, got.Stock)
		}
	}
}

// Scenario: Domain SDK: A user principal cannot call a reservation RPC (a service still needs the scope).
func TestServicePrincipalReservationScope(t *testing.T) {
	h, _ := reservationFixture()
	noScope := typedCtx("service-team-order", "service", "listing.read")
	for cname, call := range reservationCalls(h) {
		t.Run("missing scope/"+cname, func(t *testing.T) {
			st := status.Convert(call(noScope))
			if st.Code() != codes.PermissionDenied || st.Message() != "missing scope" {
				t.Fatalf("got %v %q, want PermissionDenied %q", st.Code(), st.Message(), "missing scope")
			}
		})
	}
	svc := typedCtx("service-team-order", "service", "listing.read", "listing.write")
	if _, err := h.ReserveStock(svc, &listingv1.ReserveStockRequest{ListingId: "L-1", Quantity: 1, ReservationId: "r-1"}); err != nil {
		t.Fatalf("service principal with listing.write must reserve: %v", err)
	}
}

// Scenario: Domain SDK: A seller cannot moderate.
func TestSellerCannotModerateOrReadQueue(t *testing.T) {
	sellers := map[string]context.Context{
		"owner seller": ownerCtx(),
		"other seller": otherCtx(),
		"reader":       typedCtx("seller-a", "user", "listing.read"),
		"no scopes":    typedCtx("seller-a", "user"),
	}
	for name, ctx := range sellers {
		t.Run(name, func(t *testing.T) {
			f := newMHFixture()
			_, err := f.h.ModerateListing(ctx, modReq("L-1", decApprove, "", ""))
			if st := status.Convert(err); st.Code() != codes.PermissionDenied || st.Message() != "missing scope" {
				t.Fatalf("ModerateListing got %v %q, want PermissionDenied %q", st.Code(), st.Message(), "missing scope")
			}
			_, err = f.h.ListModerationQueue(ctx, &listingv1.ListModerationQueueRequest{})
			if st := status.Convert(err); st.Code() != codes.PermissionDenied || st.Message() != "missing scope" {
				t.Fatalf("ListModerationQueue got %v %q, want PermissionDenied %q", st.Code(), st.Message(), "missing scope")
			}
			if len(f.audit.calls) != 0 {
				t.Fatalf("audit calls = %d, want 0", len(f.audit.calls))
			}
			f.assertUnchanged(ctx, t, "L-1", "pending_review")
		})
	}
}
