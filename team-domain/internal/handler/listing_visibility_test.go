package handler_test

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/handler"
	"github.com/buidangphuc/team-domain/internal/interceptor"
	"github.com/buidangphuc/team-domain/internal/repository"
	"github.com/buidangphuc/team-domain/internal/service"
)

// typedCtx injects a principal of an explicit type through the real interceptor.
func typedCtx(id, typ string, scopes ...string) context.Context {
	md := metadata.Pairs(
		"x-principal-id", id,
		"x-principal-type", typ,
		"x-principal-scopes", strings.Join(scopes, ","),
	)
	ctx := metadata.NewIncomingContext(context.Background(), md)
	var out context.Context
	_, _ = interceptor.UnaryServerInterceptor()(ctx, nil, &grpc.UnaryServerInfo{},
		func(c context.Context, _ any) (any, error) { out = c; return nil, nil })
	return out
}

func visFixture(t *testing.T) (h *handler.ListingHandler, draftID, rejectedID, pubID string) {
	t.Helper()
	repo := repository.NewInMemoryListingRepository()
	h = handler.NewListingHandler(service.NewListingService(repo), nil, nil)
	seller := typedCtx("seller-a", "user", "listing.read", "listing.write")
	mk := func(st listingv1.ListingStatus) string {
		res, err := h.CreateListing(seller, &listingv1.CreateListingRequest{Listing: &listingv1.Listing{
			Title: "t", Price: 1, Currency: "VND", Status: st, Stock: 1,
		}})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		return res.Listing.Id
	}
	draftID = mk(listingv1.ListingStatus_LISTING_STATUS_DRAFT)
	rejectedID = mk(listingv1.ListingStatus_LISTING_STATUS_REJECTED)
	pubID = mk(listingv1.ListingStatus_LISTING_STATUS_PUBLISHED)
	return
}

func code(err error) codes.Code { return status.Code(err) }

func TestGetListingVisibility(t *testing.T) {
	h, draft, rejected, pub := visFixture(t)
	anon := typedCtx("anonymous", "anonymous", "listing.read")
	otherSeller := typedCtx("seller-b", "user", "listing.read", "listing.write")
	owner := typedCtx("seller-a", "user", "listing.read", "listing.write")
	admin := typedCtx("admin-1", "user", "listing.read", "admin")
	svc := typedCtx("service-team-promotion", "service", "listing.read")

	get := func(ctx context.Context, id string) error {
		_, err := h.GetListing(ctx, &listingv1.GetListingRequest{Id: id})
		return err
	}
	for _, id := range []string{draft, rejected} {
		if c := code(get(anon, id)); c != codes.NotFound {
			t.Errorf("anonymous %s: want NotFound got %v", id, c)
		}
		if c := code(get(otherSeller, id)); c != codes.NotFound {
			t.Errorf("stranger seller %s: want NotFound got %v", id, c)
		}
		for name, ctx := range map[string]context.Context{"owner": owner, "admin": admin, "service": svc} {
			if err := get(ctx, id); err != nil {
				t.Errorf("%s %s: want ok got %v", name, id, err)
			}
		}
	}
	// Same body as an unknown id (no existence oracle).
	e1, e2 := get(anon, draft), get(anon, "does-not-exist")
	if status.Convert(e1).Message() != status.Convert(e2).Message() || code(e2) != codes.NotFound {
		t.Errorf("draft vs unknown differ: %v / %v", e1, e2)
	}
	// A user principal with service-looking scopes is not a service.
	if c := code(get(typedCtx("seller-b", "user", "listing.read", "listing.write", "inventory.write"), draft)); c != codes.NotFound {
		t.Errorf("user with service-like scopes: want NotFound got %v", c)
	}
	if err := get(anon, pub); err != nil {
		t.Errorf("anonymous published: %v", err)
	}
}

func TestListListingsVisibility(t *testing.T) {
	h, _, _, _ := visFixture(t)
	anon := typedCtx("anonymous", "anonymous", "listing.read")
	buyer := typedCtx("buyer-1", "user", "listing.read")
	admin := typedCtx("admin-1", "user", "listing.read", "admin")
	svc := typedCtx("service-team-promotion", "service", "listing.read")

	list := func(ctx context.Context, st string) (*listingv1.ListListingsResponse, error) {
		return h.ListListings(ctx, &listingv1.ListListingsRequest{Status: st})
	}
	for _, st := range []string{"", "published"} {
		res, err := list(anon, st)
		if err != nil {
			t.Fatalf("anon status=%q: %v", st, err)
		}
		for _, l := range res.Listings {
			if l.Status != listingv1.ListingStatus_LISTING_STATUS_PUBLISHED {
				t.Errorf("status=%q leaked %v", st, l.Status)
			}
		}
		if res.Page.Total != int64(len(res.Listings)) || len(res.Listings) == 0 {
			t.Errorf("status=%q total=%d len=%d", st, res.Page.Total, len(res.Listings))
		}
	}
	for _, st := range []string{"draft", "rejected"} {
		for name, ctx := range map[string]context.Context{"anonymous": anon, "buyer": buyer} {
			if _, err := list(ctx, st); code(err) != codes.PermissionDenied {
				t.Errorf("%s status=%s: want PermissionDenied got %v", name, st, err)
			}
		}
	}
	for name, ctx := range map[string]context.Context{"admin": admin, "service": svc} {
		res, err := list(ctx, "draft")
		if err != nil || len(res.Listings) != 1 {
			t.Errorf("%s draft: err=%v n=%v", name, err, res)
		}
	}
}

func TestListMyListingsKeepsOwnerDrafts(t *testing.T) {
	h, _, _, _ := visFixture(t)
	res, err := h.ListMyListings(typedCtx("seller-a", "user", "listing.write"), &listingv1.ListMyListingsRequest{})
	if err != nil || len(res.Listings) != 3 {
		t.Fatalf("ListMyListings: err=%v n=%d", err, len(res.GetListings()))
	}
}
