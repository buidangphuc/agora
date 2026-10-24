package rpc_test

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
	if _, err := vc.CreateVoucher(authCtx(t, "seller-1", "listing.write"), &promotionv1.CreateVoucherRequest{
		Code: code, Scope: promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP, Quota: 5, DiscountValue: 10,
		DiscountType: promotionv1.DiscountType_DISCOUNT_TYPE_PERCENT,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestSagaRPCsAreServiceOnly(t *testing.T) {
	vc := startRig(t, rigOpts{}).vc
	seedShopVoucher(t, vc, "SAGA")
	svc := principalCtx(t, "service-team-order", "service", "promotion.reserve")
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
		{"anonymous", principalCtx(t, "anonymous", "anonymous"), codes.Unauthenticated},
		{"buyer", authCtx(t, "buyer-1", "listing.read"), codes.PermissionDenied},
		{"seller", authCtx(t, "seller-1", "listing.write"), codes.PermissionDenied},
		{"admin", authCtx(t, "admin-1", "admin"), codes.PermissionDenied},
		{"user holding promotion.reserve", authCtx(t, "buyer-1", "promotion.reserve"), codes.PermissionDenied},
		{"service without scope", principalCtx(t, "service-x", "service", "listing.read"), codes.PermissionDenied},
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
	vc := startRig(t, rigOpts{}).vc
	seedShopVoucher(t, vc, "DUAL")
	req := func(id, buyer string) *promotionv1.ValidateAndReserveRequest {
		return &promotionv1.ValidateAndReserveRequest{
			ReservationId: id, Code: "DUAL", BuyerId: buyer, CartSubtotal: 1000, SellerId: "seller-1",
		}
	}
	buyer := authCtx(t, "buyer-1", "listing.read")
	for _, tc := range []struct {
		name string
		ctx  context.Context
		req  *promotionv1.ValidateAndReserveRequest
		want codes.Code
	}{
		{"anonymous", principalCtx(t, "anonymous", "anonymous"), req("preview:anonymous:x", ""), codes.Unauthenticated},
		{"user own preview namespace", buyer, req("preview:buyer-1:DUAL", "someone-else"), codes.OK},
		{"user bare id", buyer, req("order-1", "buyer-1"), codes.PermissionDenied},
		{"user other's namespace", buyer, req("preview:buyer-2:DUAL", "buyer-2"), codes.PermissionDenied},
		{"service without scope", principalCtx(t, "service-x", "service", "listing.read"), req("o2", "b1"), codes.PermissionDenied},
		{"service with scope", principalCtx(t, "service-team-order", "service", "promotion.reserve"), req("o2", "b1"), codes.OK},
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
	sub := startRig(t, rigOpts{listings: ownedListings{"l1": "seller-1"}}).sub
	plans, err := sub.ListPlans(t.Context(), &promotionv1.ListPlansRequest{})
	if err != nil || len(plans.GetPlans()) == 0 {
		t.Fatalf("ListPlans: %v", err)
	}
	planID := plans.GetPlans()[0].GetId()

	for _, tc := range []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"anonymous", principalCtx(t, "anonymous", "anonymous"), codes.Unauthenticated},
		{"buyer", authCtx(t, "buyer-1", "listing.read"), codes.PermissionDenied},
		{"seller", authCtx(t, "seller-1", "listing.write"), codes.OK},
		{"admin", authCtx(t, "admin-1", "listing.write", "admin"), codes.OK},
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
		{"anonymous", principalCtx(t, "anonymous", "anonymous"), other, codes.Unauthenticated},
		{"user other seller", authCtx(t, "seller-1", "listing.write"), other, codes.PermissionDenied},
		{"unspecified-type other seller", principalCtx(t, "seller-1", "", "listing.write"), other, codes.PermissionDenied},
		{"admin other seller", authCtx(t, "admin-1", "admin"), other, codes.PermissionDenied},
		{"user self", authCtx(t, "seller-1", "listing.write"), &promotionv1.GetEntitlementsRequest{}, codes.OK},
		{"service other seller", principalCtx(t, "service-team-order", "service", "identity.read"), other, codes.OK},
	} {
		if _, err := sub.GetEntitlements(tc.ctx, tc.req); code(err) != tc.want {
			t.Errorf("entitlements %s: got %v, want %v (err=%v)", tc.name, code(err), tc.want, err)
		}
	}
}

func TestCreateAdCampaignAuthzAndCaps(t *testing.T) {
	ads := startRig(t, rigOpts{}).ads
	mk := func(listing string, bid, budget int64) *promotionv1.CreateAdCampaignRequest {
		return &promotionv1.CreateAdCampaignRequest{ListingId: listing, Bid: bid, Budget: budget}
	}
	seller := authCtx(t, "seller-1", "listing.write")
	for _, tc := range []struct {
		name string
		ctx  context.Context
		req  *promotionv1.CreateAdCampaignRequest
		want codes.Code
	}{
		{"anonymous", principalCtx(t, "anonymous", "anonymous"), mk("l1", 1, 1), codes.Unauthenticated},
		{"buyer", authCtx(t, "buyer-1", "listing.read"), mk("l1", 1, 1), codes.PermissionDenied},
		{"seller own listing", seller, mk("l1", 10, 100), codes.OK},
		{"seller foreign listing", seller, mk("l2", 10, 100), codes.PermissionDenied},
		{"seller unknown listing", seller, mk("nope", 10, 100), codes.InvalidArgument},
		{"admin foreign listing", authCtx(t, "admin-1", "listing.write", "admin"), mk("l2", 10, 100), codes.OK},
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
	r := startRig(t, rigOpts{listings: failingListings{}})
	fc, ads := r.fc, r.ads
	seller := authCtx(t, "seller-1", "listing.write")
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

func firstPlanID(t *testing.T, sub promotionv1.SubscriptionServiceClient) string {
	t.Helper()
	plans, err := sub.ListPlans(t.Context(), &promotionv1.ListPlansRequest{})
	if err != nil || len(plans.GetPlans()) == 0 {
		t.Fatalf("ListPlans: %v", err)
	}
	return plans.GetPlans()[0].GetId()
}

func TestSubscribeAdminWithoutListingWriteDenied(t *testing.T) {
	r := startRig(t, rigOpts{})
	planID := firstPlanID(t, r.sub)
	if _, err := r.sub.Subscribe(authCtx(t, "seller-1", "listing.write"), &promotionv1.SubscribeRequest{PlanId: planID}); err != nil {
		t.Fatalf("listing.write subscribe: %v", err)
	}
	if _, err := r.sub.Subscribe(authCtx(t, "admin-1", "admin"), &promotionv1.SubscribeRequest{PlanId: planID}); code(err) != codes.PermissionDenied {
		t.Fatalf("admin without listing.write: got %v, want PermissionDenied (err=%v)", code(err), err)
	}
}

func TestCreateAdCampaignAdminWithoutListingWriteDenied(t *testing.T) {
	rec := &recordingAudit{}
	r := startRig(t, rigOpts{audit: rec})
	_, err := r.ads.CreateAdCampaign(authCtx(t, "admin-1", "admin"), &promotionv1.CreateAdCampaignRequest{ListingId: "l1", Bid: 10, Budget: 100})
	if code(err) != codes.PermissionDenied {
		t.Fatalf("got %v, want PermissionDenied (err=%v)", code(err), err)
	}
	rec.requireNone(t, "admin without listing.write")
	slots, err := r.ads.ListSponsoredSlots(t.Context(), &promotionv1.ListSponsoredSlotsRequest{})
	if err != nil || len(slots.GetListingIds()) != 0 {
		t.Fatalf("no campaign may exist: slots=%v err=%v", slots.GetListingIds(), err)
	}
}

func TestAnonymousIsUnauthenticated(t *testing.T) {
	r := startRig(t, rigOpts{})
	if _, err := r.vc.CreateVoucher(anonCtx(t), platformVoucher("ANON", promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP)); code(err) != codes.Unauthenticated {
		t.Errorf("CreateVoucher: got %v, want Unauthenticated", code(err))
	}
	c := seedCampaign(t, r, "", 5)
	if _, err := r.fc.ReserveFlashSaleStock(anonCtx(t), reserveReq("anon-1", c, 1)); code(err) != codes.Unauthenticated {
		t.Errorf("ReserveFlashSaleStock: got %v, want Unauthenticated", code(err))
	}
}

func TestUserWithServiceScopeIsDenied(t *testing.T) {
	r := startRig(t, rigOpts{})
	_, err := r.vc.CommitReservation(authCtx(t, "buyer-1", "promotion.reserve"), &promotionv1.CommitReservationRequest{ReservationId: "o1"})
	if code(err) != codes.PermissionDenied {
		t.Fatalf("got %v, want PermissionDenied (err=%v)", code(err), err)
	}
}

func TestSellerCannotCreatePlatformVoucher(t *testing.T) {
	rec := &recordingAudit{err: errors.New("audit down")}
	r := startRig(t, rigOpts{audit: rec})
	_, err := r.vc.CreateVoucher(sellerCtx(t), platformVoucher("SELLER-PLAT", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM))
	if code(err) != codes.PermissionDenied {
		t.Fatalf("got %v, want PermissionDenied not Unavailable (err=%v)", code(err), err)
	}
	rec.requireNone(t, "seller platform voucher")
	if _, err := r.vouchers.GetByCode(t.Context(), "SELLER-PLAT"); err == nil {
		t.Fatal("voucher was stored")
	}
}

func TestOrderServicePrincipalIsServed(t *testing.T) {
	r := startRig(t, rigOpts{})
	c := seedCampaign(t, r, "", 5)
	resp, err := r.fc.ReserveFlashSaleStock(orderSvcCtx(t), reserveReq("o1", c, 2))
	if err != nil || !resp.GetReserved() {
		t.Fatalf("reserve: %v %+v, want reserved=true", err, resp)
	}
}

func TestPublicReadsNeedNoPrincipal(t *testing.T) {
	r := startRig(t, rigOpts{})
	ctx := t.Context()
	for name, tc := range map[string]struct {
		call func() error
		want codes.Code
	}{
		"GetVoucher": {func() error {
			_, err := r.vc.GetVoucher(ctx, &promotionv1.GetVoucherRequest{Code: "NOPE"})
			return err
		}, codes.NotFound},
		"ListVouchers": {func() error {
			_, err := r.vc.ListVouchers(ctx, &promotionv1.ListVouchersRequest{})
			return err
		}, codes.OK},
		"GetActiveFlashSale": {func() error {
			_, err := r.fc.GetActiveFlashSale(ctx, &promotionv1.GetActiveFlashSaleRequest{ListingId: "l1"})
			return err
		}, codes.OK},
		"ListActiveCampaigns": {func() error {
			_, err := r.fc.ListActiveCampaigns(ctx, &promotionv1.ListActiveCampaignsRequest{})
			return err
		}, codes.OK},
		"GetFlashSaleStock": {func() error {
			_, err := r.fc.GetFlashSaleStock(ctx, &promotionv1.GetFlashSaleStockRequest{CampaignId: "nope"})
			return err
		}, codes.NotFound},
		"ListPlans": {func() error {
			_, err := r.sub.ListPlans(ctx, &promotionv1.ListPlansRequest{})
			return err
		}, codes.OK},
		"ListSponsoredSlots": {func() error {
			_, err := r.ads.ListSponsoredSlots(ctx, &promotionv1.ListSponsoredSlotsRequest{})
			return err
		}, codes.OK},
	} {
		if err := tc.call(); code(err) != tc.want {
			t.Errorf("%s: got %v, want %v (err=%v)", name, code(err), tc.want, err)
		}
	}
}
