package rpc_test

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/rpc"
	"github.com/buidangphuc/team-domain/internal/store/storetest"
)

type sfCapEnt struct {
	ent   listing.Entitlements
	err   error
	calls int
}

func (f *sfCapEnt) Get(context.Context, string) (listing.Entitlements, error) {
	f.calls++
	return f.ent, f.err
}

func sfCapHandler(featured int64) (*rpc.ListingHandler, *sfCapEnt) {
	ent := &sfCapEnt{ent: listing.Entitlements{FeaturedListings: featured}}
	sf := listing.NewStorefrontService(storetest.NewInMemoryStorefrontRepository(), "").WithEntitlements(ent)
	h := rpc.NewListingHandler(listing.NewListingService(storetest.NewInMemoryListingRepository()), nil, nil).WithStorefront(sf)
	return h, ent
}

func sfCapReq(n int) *listingv1.UpsertStorefrontRequest {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("l-%d", i)
	}
	return &listingv1.UpsertStorefrontRequest{Storefront: &listingv1.Storefront{Slug: "shop", FeaturedListingIds: ids}}
}

func TestRPCStorefrontFeaturedAbovePlanCapRejected(t *testing.T) {
	h, _ := sfCapHandler(5)
	_, err := h.UpsertStorefront(sellerCtx("s1"), sfCapReq(6))
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("code = %v, want ResourceExhausted (%v)", status.Code(err), err)
	}
	if r := errorReason(t, err); r != "PLAN_FEATURED_LIMIT" {
		t.Errorf("reason = %q, want PLAN_FEATURED_LIMIT", r)
	}
	if _, gerr := h.GetStorefront(sellerCtx("s1"), &listingv1.GetStorefrontRequest{SellerId: "s1"}); status.Code(gerr) != codes.NotFound {
		t.Errorf("rejected write persisted: %v", gerr)
	}
}

func TestRPCStorefrontFeaturedAtPlanCapAccepted(t *testing.T) {
	h, _ := sfCapHandler(5)
	res, err := h.UpsertStorefront(sellerCtx("s1"), sfCapReq(5))
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if n := len(res.GetStorefront().GetFeaturedListingIds()); n != 5 {
		t.Errorf("ids = %d, want 5", n)
	}
}

func TestRPCStorefrontFreeSellerCannotFeature(t *testing.T) {
	h, _ := sfCapHandler(0)
	_, err := h.UpsertStorefront(sellerCtx("s1"), sfCapReq(1))
	if status.Code(err) != codes.ResourceExhausted || errorReason(t, err) != "PLAN_FEATURED_LIMIT" {
		t.Fatalf("err = %v reason %q", err, errorReason(t, err))
	}
}

func TestRPCStorefrontFeaturedFailsClosed(t *testing.T) {
	h, ent := sfCapHandler(5)
	ent.err = fmt.Errorf("%w: down", listing.ErrEntitlementsUnavailable)
	_, err := h.UpsertStorefront(sellerCtx("s1"), sfCapReq(1))
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("code = %v, want Unavailable", status.Code(err))
	}
	if _, err := h.UpsertStorefront(sellerCtx("s1"), sfCapReq(0)); err != nil {
		t.Errorf("no featured ids must not need the plan: %v", err)
	}
}
