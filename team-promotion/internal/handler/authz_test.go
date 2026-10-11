package handler_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-promotion/generated/platform/listing/v1"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
)

func seedShopVoucher(t *testing.T, vc promotionv1.VoucherServiceClient, code string) {
	t.Helper()
	if _, err := vc.CreateVoucher(authCtx("seller-1", "listing.write"), &promotionv1.CreateVoucherRequest{
		Code: code, Scope: promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP, Quota: 5, DiscountValue: 10,
		DiscountType: promotionv1.DiscountType_DISCOUNT_TYPE_PERCENT,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestSagaRPCsAreServiceOnly(t *testing.T) {
	vc, _ := startServer(t)
	seedShopVoucher(t, vc, "SAGA")
	svc := principalCtx("service-team-order", "service", "promotion.reserve")
	if _, err := vc.ValidateAndReserve(svc, &promotionv1.ValidateAndReserveRequest{
		ReservationId: "o1", Code: "SAGA", BuyerId: "b1", CartSubtotal: 1000, SellerId: "seller-1",
	}); err != nil {
		t.Fatalf("service reserve: %v", err)
	}

	for _, tc := range []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"anonymous", principalCtx("anonymous", "anonymous"), codes.Unauthenticated},
		{"buyer", authCtx("buyer-1", "listing.read"), codes.PermissionDenied},
		{"seller", authCtx("seller-1", "listing.write"), codes.PermissionDenied},
		{"admin", authCtx("admin-1", "admin"), codes.PermissionDenied},
		{"user holding promotion.reserve", authCtx("buyer-1", "promotion.reserve"), codes.PermissionDenied},
		{"service without scope", principalCtx("service-x", "service", "listing.read"), codes.PermissionDenied},
	} {
		if _, err := vc.CommitReservation(tc.ctx, &promotionv1.CommitReservationRequest{ReservationId: "o1"}); code(err) != tc.want {
			t.Errorf("commit %s: got %v, want %v", tc.name, code(err), tc.want)
		}
		if _, err := vc.ReleaseReservation(tc.ctx, &promotionv1.ReleaseReservationRequest{ReservationId: "o1"}); code(err) != tc.want {
			t.Errorf("release %s: got %v, want %v", tc.name, code(err), tc.want)
		}
	}
	// The hold survived every refused attempt and still commits for the service.
	r, err := vc.CommitReservation(svc, &promotionv1.CommitReservationRequest{ReservationId: "o1"})
	if err != nil || !r.GetCommitted() {
		t.Fatalf("service commit: %v %+v", err, r)
	}
}

func TestValidateAndReserveDualMode(t *testing.T) {
	vc, _ := startServer(t)
	seedShopVoucher(t, vc, "DUAL")
	req := func(id, buyer string) *promotionv1.ValidateAndReserveRequest {
		return &promotionv1.ValidateAndReserveRequest{
			ReservationId: id, Code: "DUAL", BuyerId: buyer, CartSubtotal: 1000, SellerId: "seller-1",
		}
	}
	buyer := authCtx("buyer-1", "listing.read")
	for _, tc := range []struct {
		name string
		ctx  context.Context
		req  *promotionv1.ValidateAndReserveRequest
		want codes.Code
	}{
		{"anonymous", principalCtx("anonymous", "anonymous"), req("preview:anonymous:x", ""), codes.Unauthenticated},
		{"user own preview namespace", buyer, req("preview:buyer-1:DUAL", "someone-else"), codes.OK},
		{"user bare id", buyer, req("order-1", "buyer-1"), codes.PermissionDenied},
		{"user other's namespace", buyer, req("preview:buyer-2:DUAL", "buyer-2"), codes.PermissionDenied},
		{"service without scope", principalCtx("service-x", "service", "listing.read"), req("o2", "b1"), codes.PermissionDenied},
		{"service with scope", principalCtx("service-team-order", "service", "promotion.reserve"), req("o2", "b1"), codes.OK},
	} {
		if _, err := vc.ValidateAndReserve(tc.ctx, tc.req); code(err) != tc.want {
			t.Errorf("%s: got %v, want %v (err=%v)", tc.name, code(err), tc.want, err)
		}
	}
	// A user preview hold cannot be committed (quota untouched): commit is service-only.
	if _, err := vc.CommitReservation(buyer, &promotionv1.CommitReservationRequest{ReservationId: "preview:buyer-1:DUAL"}); code(err) != codes.PermissionDenied {
		t.Errorf("user commit of own preview: got %v, want PermissionDenied", code(err))
	}
}

func TestSubscribeAndEntitlementsAuthz(t *testing.T) {
	_, _, sub, _ := startAll(t, ownedListings{"l1": "seller-1"})
	plans, err := sub.ListPlans(context.Background(), &promotionv1.ListPlansRequest{})
	if err != nil || len(plans.GetPlans()) == 0 {
		t.Fatalf("ListPlans: %v", err)
	}
	planID := plans.GetPlans()[0].GetId()

	for _, tc := range []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"anonymous", principalCtx("anonymous", "anonymous"), codes.Unauthenticated},
		{"buyer", authCtx("buyer-1", "listing.read"), codes.PermissionDenied},
		{"seller", authCtx("seller-1", "listing.write"), codes.OK},
		{"admin", authCtx("admin-1", "listing.write", "admin"), codes.OK},
	} {
		if _, err := sub.Subscribe(tc.ctx, &promotionv1.SubscribeRequest{PlanId: planID}); code(err) != tc.want {
			t.Errorf("subscribe %s: got %v, want %v (err=%v)", tc.name, code(err), tc.want, err)
		}
	}

	other := &promotionv1.GetEntitlementsRequest{SellerId: "seller-2"}
	for _, tc := range []struct {
		name string
		ctx  context.Context
		req  *promotionv1.GetEntitlementsRequest
		want codes.Code
	}{
		{"anonymous", principalCtx("anonymous", "anonymous"), other, codes.Unauthenticated},
		{"user other seller", authCtx("seller-1", "listing.write"), other, codes.PermissionDenied},
		{"unspecified-type other seller", principalCtx("seller-1", "", "listing.write"), other, codes.PermissionDenied},
		{"admin other seller", authCtx("admin-1", "admin"), other, codes.PermissionDenied},
		{"user self", authCtx("seller-1", "listing.write"), &promotionv1.GetEntitlementsRequest{}, codes.OK},
		{"service other seller", principalCtx("service-team-order", "service", "identity.read"), other, codes.OK},
	} {
		if _, err := sub.GetEntitlements(tc.ctx, tc.req); code(err) != tc.want {
			t.Errorf("entitlements %s: got %v, want %v (err=%v)", tc.name, code(err), tc.want, err)
		}
	}
}

func TestCreateAdCampaignAuthzAndCaps(t *testing.T) {
	_, _, _, ads := startAll(t, ownedListings{"l1": "seller-1", "l2": "seller-2"})
	mk := func(listing string, bid, budget int64) *promotionv1.CreateAdCampaignRequest {
		return &promotionv1.CreateAdCampaignRequest{ListingId: listing, Bid: bid, Budget: budget}
	}
	seller := authCtx("seller-1", "listing.write")
	for _, tc := range []struct {
		name string
		ctx  context.Context
		req  *promotionv1.CreateAdCampaignRequest
		want codes.Code
	}{
		{"anonymous", principalCtx("anonymous", "anonymous"), mk("l1", 1, 1), codes.Unauthenticated},
		{"buyer", authCtx("buyer-1", "listing.read"), mk("l1", 1, 1), codes.PermissionDenied},
		{"seller own listing", seller, mk("l1", 10, 100), codes.OK},
		{"seller foreign listing", seller, mk("l2", 10, 100), codes.PermissionDenied},
		{"seller unknown listing", seller, mk("nope", 10, 100), codes.InvalidArgument},
		{"admin foreign listing", authCtx("admin-1", "listing.write", "admin"), mk("l2", 10, 100), codes.OK},
		{"bid at cap", seller, mk("l1", 1000, 100), codes.OK},
		{"bid above cap", seller, mk("l1", 1001, 100), codes.InvalidArgument},
		{"budget above cap", seller, mk("l1", 10, 5001), codes.InvalidArgument},
	} {
		if _, err := ads.CreateAdCampaign(tc.ctx, tc.req); code(err) != tc.want {
			t.Errorf("%s: got %v, want %v (err=%v)", tc.name, code(err), tc.want, err)
		}
	}
}

// failingListings simulates a team-domain outage with a revealing error message.
type failingListings struct{}

func (failingListings) GetListing(context.Context, *listingv1.GetListingRequest, ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	return nil, errors.New("dial tcp 10.0.0.5:50051: connection refused")
}

func TestUpstreamOutageIsUnavailableAndGeneric(t *testing.T) {
	_, fc, _, ads := startAll(t, failingListings{})
	seller := authCtx("seller-1", "listing.write")
	_, err1 := ads.CreateAdCampaign(seller, &promotionv1.CreateAdCampaignRequest{ListingId: "l1", Bid: 1, Budget: 1})
	_, err2 := fc.CreateCampaign(seller, &promotionv1.CreateCampaignRequest{ListingId: "l1", SalePrice: 1, StockCap: 1})
	for name, err := range map[string]error{"ad campaign": err1, "flash sale": err2} {
		st, _ := status.FromError(err)
		if st.Code() != codes.Unavailable {
			t.Errorf("%s: code = %v, want Unavailable", name, st.Code())
		}
		if got := st.Message(); got != "listing lookup failed" {
			t.Errorf("%s: message leaks detail: %q", name, got)
		}
	}
}
