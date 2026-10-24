package rpc_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
)

func requireAuditUnavailable(t *testing.T, err error) {
	t.Helper()
	st := status.Convert(err)
	if st.Code() != codes.Unavailable || st.Message() != auditx.Message {
		t.Fatalf("got %v %q, want Unavailable %q", st.Code(), st.Message(), auditx.Message)
	}
}

// Scenario: Unspecified scope is audited like platform (and platform itself,
// carrying the verified admin id as actor).
func TestCreateVoucherUnspecifiedScopeAudited(t *testing.T) {
	for name, scope := range map[string]promotionv1.VoucherScope{
		"unspecified": promotionv1.VoucherScope_VOUCHER_SCOPE_UNSPECIFIED,
		"platform":    promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM,
	} {
		t.Run(name, func(t *testing.T) {
			rec := &recordingAudit{}
			vc := startRig(t, rigOpts{audit: rec}).vc
			if _, err := vc.CreateVoucher(adminCtx(t), platformVoucher("PLAT10", scope)); err != nil {
				t.Fatalf("CreateVoucher: %v", err)
			}
			evs := rec.got()
			if len(evs) != 1 {
				t.Fatalf("want 1 audit event, got %d", len(evs))
			}
			ev := evs[0]
			if ev.Action != "voucher.platform_create" || ev.TargetType != "voucher" || ev.TargetID != "PLAT10" || ev.ActorID != "admin-1" {
				t.Errorf("event = %+v", ev)
			}
			if ev.Metadata["discount_type"] != "DISCOUNT_TYPE_PERCENT" || ev.Metadata["value"] != "10" {
				t.Errorf("metadata = %v", ev.Metadata)
			}
		})
	}
}

// Scenario: Non-admin cannot create a platform voucher and nothing is audited.
func TestCreateVoucherPlatformDeniedWritesNoAudit(t *testing.T) {
	rec := &recordingAudit{}
	vc := startRig(t, rigOpts{audit: rec}).vc
	platform := promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"seller", sellerCtx(t), codes.PermissionDenied},
		{"buyer", buyerCtx(t), codes.PermissionDenied},
		{"anonymous", anonCtx(t), codes.Unauthenticated},
	} {
		if _, err := vc.CreateVoucher(tc.ctx, platformVoucher("DENY-"+tc.name, platform)); code(err) != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, code(err), tc.want)
		}
	}
	rec.requireNone(t, "refused callers")
}

// Scenario: Invalid platform voucher writes no event.
func TestCreateVoucherInvalidWritesNoAudit(t *testing.T) {
	rec := &recordingAudit{}
	vc := startRig(t, rigOpts{audit: rec}).vc
	_, err := vc.CreateVoucher(adminCtx(t), platformVoucher("", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM))
	if code(err) != codes.InvalidArgument {
		t.Fatalf("got %v, want InvalidArgument", code(err))
	}
	rec.requireNone(t, "empty code")
}

// Scenario: A failing audit writer refuses the audited action with one message
// (handler maps Unavailable through unchanged; nothing persisted).
func TestCreateVoucherAuditDownUnavailable(t *testing.T) {
	platform := promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM
	for name, audit := range map[string]*recordingAudit{
		"audit failing": {err: errors.New("dial tcp 10.0.0.9:50066: connection refused")},
		"audit timeout": {err: context.DeadlineExceeded},
	} {
		t.Run(name, func(t *testing.T) {
			vc := startRig(t, rigOpts{audit: audit}).vc
			_, err := vc.CreateVoucher(adminCtx(t), platformVoucher("PLAT-DOWN", platform))
			requireAuditUnavailable(t, err)
			if _, err := vc.GetVoucher(adminCtx(t), &promotionv1.GetVoucherRequest{Code: "PLAT-DOWN"}); code(err) != codes.NotFound {
				t.Fatalf("voucher must not exist, GetVoucher = %v", code(err))
			}
		})
	}
}

// Scenario: Admin creates a flash sale with an audit event (handler half: the
// verified admin id is the actor, the response id is the target).
func TestCreateCampaignAdminAudited(t *testing.T) {
	rec := &recordingAudit{}
	fc := startRig(t, rigOpts{audit: rec}).fc
	resp, err := fc.CreateCampaign(adminCtx(t), &promotionv1.CreateCampaignRequest{ListingId: "l2", SalePrice: 1000, StockCap: 5})
	if err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}
	evs := rec.got()
	if len(evs) != 1 {
		t.Fatalf("want 1 audit event, got %d", len(evs))
	}
	ev := evs[0]
	if ev.Action != "flash_sale.admin_create" || ev.TargetType != "flash_sale" || ev.ActorID != "admin-1" ||
		ev.TargetID != resp.GetCampaign().GetId() || ev.TargetID == "" || ev.Metadata["listing_id"] != "l2" {
		t.Errorf("event = %+v, response id = %q", ev, resp.GetCampaign().GetId())
	}
}

// Scenario: Seller creates a flash sale on their own listing (not audited, and
// unaffected by audit availability); seller paths ignore audit availability.
func TestCreateCampaignSellerNotAudited(t *testing.T) {
	rec := &recordingAudit{err: errors.New("audit down")}
	fc := startRig(t, rigOpts{audit: rec}).fc
	if _, err := fc.CreateCampaign(sellerCtx(t), &promotionv1.CreateCampaignRequest{ListingId: "l1", SalePrice: 1000, StockCap: 5}); err != nil {
		t.Fatalf("seller campaign: %v", err)
	}
	rec.requireNone(t, "seller flash sale")
}

// Scenario: Audit down blocks an admin flash sale.
func TestCreateCampaignAuditDownUnavailable(t *testing.T) {
	for name, audit := range map[string]auditx.Writer{"failing": &recordingAudit{err: errors.New("down")}} {
		t.Run(name, func(t *testing.T) {
			fc := startRig(t, rigOpts{audit: audit}).fc
			_, err := fc.CreateCampaign(adminCtx(t), &promotionv1.CreateCampaignRequest{ListingId: "l1", SalePrice: 1000, StockCap: 5})
			requireAuditUnavailable(t, err)
			got, err := fc.GetActiveFlashSale(adminCtx(t), &promotionv1.GetActiveFlashSaleRequest{ListingId: "l1"})
			if err != nil || got.GetActive() {
				t.Fatalf("no sale may exist: active=%v err=%v", got.GetActive(), err)
			}
		})
	}
}

// Scenario: Invalid campaign writes no event.
func TestCreateCampaignInvalidWritesNoAudit(t *testing.T) {
	rec := &recordingAudit{}
	fc := startRig(t, rigOpts{audit: rec}).fc
	_, err := fc.CreateCampaign(adminCtx(t), &promotionv1.CreateCampaignRequest{ListingId: "", SalePrice: 1000, StockCap: 5})
	if code(err) != codes.InvalidArgument {
		t.Fatalf("got %v, want InvalidArgument", code(err))
	}
	rec.requireNone(t, "empty listing_id")
}

// Scenario: Refused callers write no flash-sale event.
func TestCreateCampaignDeniedWritesNoAudit(t *testing.T) {
	rec := &recordingAudit{}
	fc := startRig(t, rigOpts{audit: rec}).fc
	for _, tc := range []struct {
		name    string
		ctx     context.Context
		listing string
		want    codes.Code
	}{
		{"buyer", buyerCtx(t), "l1", codes.PermissionDenied},
		{"anonymous", anonCtx(t), "l1", codes.Unauthenticated},
		{"seller foreign listing", sellerCtx(t), "l2", codes.PermissionDenied},
	} {
		_, err := fc.CreateCampaign(tc.ctx, &promotionv1.CreateCampaignRequest{ListingId: tc.listing, SalePrice: 1000, StockCap: 5})
		if code(err) != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, code(err), tc.want)
		}
	}
	rec.requireNone(t, "refused callers")
}

// Scenario: Admin creates an ad campaign with an audit event (handler half).
func TestCreateAdCampaignAdminAudited(t *testing.T) {
	rec := &recordingAudit{}
	ads := startRig(t, rigOpts{audit: rec}).ads
	resp, err := ads.CreateAdCampaign(adminCtx(t), &promotionv1.CreateAdCampaignRequest{ListingId: "l2", Bid: 100, Budget: 5000})
	if err != nil {
		t.Fatalf("CreateAdCampaign: %v", err)
	}
	evs := rec.got()
	if len(evs) != 1 {
		t.Fatalf("want 1 audit event, got %d", len(evs))
	}
	ev, c := evs[0], resp.GetCampaign()
	if ev.Action != "ad_campaign.admin_create" || ev.TargetType != "ad_campaign" || ev.ActorID != "admin-1" ||
		ev.TargetID != c.GetId() || ev.TargetID == "" {
		t.Errorf("event = %+v, response id = %q", ev, c.GetId())
	}
	if ev.Metadata["seller_id"] != c.GetSellerId() || ev.Metadata["listing_id"] != "l2" || ev.Metadata["budget"] != "5000" {
		t.Errorf("metadata = %v, campaign seller = %q", ev.Metadata, c.GetSellerId())
	}
}

// Scenario: Seller creates an ad campaign on their own listing (not audited).
func TestCreateAdCampaignSellerNotAudited(t *testing.T) {
	rec := &recordingAudit{err: errors.New("audit down")}
	ads := startRig(t, rigOpts{audit: rec}).ads
	if _, err := ads.CreateAdCampaign(sellerCtx(t), &promotionv1.CreateAdCampaignRequest{ListingId: "l1", Bid: 10, Budget: 100}); err != nil {
		t.Fatalf("seller ad campaign: %v", err)
	}
	rec.requireNone(t, "seller ad campaign")
}

// Scenario: Audit down blocks an admin ad campaign.
func TestCreateAdCampaignAuditDownUnavailable(t *testing.T) {
	for name, audit := range map[string]auditx.Writer{"failing": &recordingAudit{err: errors.New("down")}} {
		t.Run(name, func(t *testing.T) {
			ads := startRig(t, rigOpts{audit: audit}).ads
			_, err := ads.CreateAdCampaign(adminCtx(t), &promotionv1.CreateAdCampaignRequest{ListingId: "l1", Bid: 10, Budget: 100})
			requireAuditUnavailable(t, err)
			slots, err := ads.ListSponsoredSlots(anonCtx(t), &promotionv1.ListSponsoredSlotsRequest{})
			if err != nil || len(slots.GetListingIds()) != 0 {
				t.Fatalf("no campaign may exist: slots=%v err=%v", slots.GetListingIds(), err)
			}
		})
	}
}

// Scenario: Invalid campaign writes no event (ad campaign budget above the cap).
func TestCreateAdCampaignInvalidWritesNoAudit(t *testing.T) {
	rec := &recordingAudit{}
	ads := startRig(t, rigOpts{audit: rec}).ads
	for name, req := range map[string]*promotionv1.CreateAdCampaignRequest{
		"empty listing":    {ListingId: "", Bid: 10, Budget: 100},
		"budget above cap": {ListingId: "l1", Bid: 10, Budget: 5001},
	} {
		if _, err := ads.CreateAdCampaign(adminCtx(t), req); code(err) != codes.InvalidArgument {
			t.Errorf("%s: got %v, want InvalidArgument", name, code(err))
		}
	}
	rec.requireNone(t, "invalid ad campaigns")
}

// Scenario: Refused callers write no ad-campaign event.
func TestCreateAdCampaignDeniedWritesNoAudit(t *testing.T) {
	rec := &recordingAudit{}
	ads := startRig(t, rigOpts{audit: rec}).ads
	for _, tc := range []struct {
		name    string
		ctx     context.Context
		listing string
	}{
		{"buyer without listing.write", buyerCtx(t), "l1"},
		{"seller foreign listing", sellerCtx(t), "l2"},
	} {
		_, err := ads.CreateAdCampaign(tc.ctx, &promotionv1.CreateAdCampaignRequest{ListingId: tc.listing, Bid: 10, Budget: 100})
		if code(err) != codes.PermissionDenied {
			t.Errorf("%s: got %v, want PermissionDenied", tc.name, code(err))
		}
	}
	rec.requireNone(t, "refused callers")
}

// Scenario: Seller paths unaffected with audit down
func TestSellerPathsIgnoreAuditAvailability(t *testing.T) {
	rec := &recordingAudit{err: errors.New("audit down")}
	r := startRig(t, rigOpts{audit: rec})
	vc, fc, ads := r.vc, r.fc, r.ads
	if _, err := vc.CreateVoucher(sellerCtx(t), platformVoucher("SHOP-NIL", promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP)); err != nil {
		t.Errorf("shop voucher: %v", err)
	}
	if _, err := fc.CreateCampaign(sellerCtx(t), &promotionv1.CreateCampaignRequest{ListingId: "l1", SalePrice: 1000, StockCap: 5}); err != nil {
		t.Errorf("flash sale: %v", err)
	}
	if _, err := ads.CreateAdCampaign(sellerCtx(t), &promotionv1.CreateAdCampaignRequest{ListingId: "l1", Bid: 10, Budget: 100}); err != nil {
		t.Errorf("ad campaign: %v", err)
	}
	rec.requireNone(t, "seller paths")
}

// Scenario: Shop voucher audit: admin shop voucher is audited by the handler with
// the admin as actor.
func TestCreateVoucherShopAdminAudited(t *testing.T) {
	rec := &recordingAudit{}
	vc := startRig(t, rigOpts{audit: rec}).vc
	if _, err := vc.CreateVoucher(adminCtx(t), platformVoucher("SHOP-ADM10", promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP)); err != nil {
		t.Fatalf("CreateVoucher: %v", err)
	}
	evs := rec.got()
	if len(evs) != 1 {
		t.Fatalf("want 1 audit event, got %d: %+v", len(evs), evs)
	}
	ev := evs[0]
	if ev.Action != "voucher.admin_create" || ev.TargetType != "voucher" || ev.TargetID != "SHOP-ADM10" || ev.ActorID != "admin-1" {
		t.Errorf("event = %+v", ev)
	}
	if ev.Metadata["discount_type"] != "DISCOUNT_TYPE_PERCENT" || ev.Metadata["value"] != "10" || ev.Metadata["seller_id"] != "admin-1" {
		t.Errorf("metadata = %v", ev.Metadata)
	}
}

// Scenario: Shop voucher audit: audit down blocks an admin shop voucher (handler
// passes Unavailable through unchanged; nothing persisted).
func TestCreateVoucherShopAdminAuditDownUnavailable(t *testing.T) {
	for name, audit := range map[string]auditx.Writer{"failing": &recordingAudit{err: errors.New("down")}} {
		t.Run(name, func(t *testing.T) {
			vc := startRig(t, rigOpts{audit: audit}).vc
			_, err := vc.CreateVoucher(adminCtx(t), platformVoucher("SHOP-DOWN", promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP))
			requireAuditUnavailable(t, err)
			if _, err := vc.GetVoucher(adminCtx(t), &promotionv1.GetVoucherRequest{Code: "SHOP-DOWN"}); code(err) != codes.NotFound {
				t.Fatalf("voucher must not exist, GetVoucher = %v", code(err))
			}
		})
	}
}

// Scenario: Shop voucher audit: seller path needs no audit writer.
func TestCreateVoucherShopSellerWritesNoAudit(t *testing.T) {
	shop := promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP
	rec := &recordingAudit{err: errors.New("audit down")}
	vc := startRig(t, rigOpts{audit: rec}).vc
	if _, err := vc.CreateVoucher(sellerCtx(t), platformVoucher("SHOP-SELL10", shop)); err != nil {
		t.Fatalf("seller shop voucher: %v", err)
	}
	rec.requireNone(t, "seller shop voucher")
}

// Scenario: Shop voucher audit: refused callers write nothing.
func TestCreateVoucherShopRefusedWritesNoAudit(t *testing.T) {
	rec := &recordingAudit{}
	vc := startRig(t, rigOpts{audit: rec}).vc
	shop := promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"buyer", buyerCtx(t), codes.PermissionDenied},
		{"anonymous", anonCtx(t), codes.Unauthenticated},
	} {
		if _, err := vc.CreateVoucher(tc.ctx, platformVoucher("SHOP-DENY-"+tc.name, shop)); code(err) != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, code(err), tc.want)
		}
	}
	rec.requireNone(t, "refused callers")
}

// Scenario: Shop voucher audit: invalid request writes nothing.
func TestCreateVoucherShopAdminInvalidWritesNoAudit(t *testing.T) {
	rec := &recordingAudit{}
	vc := startRig(t, rigOpts{audit: rec}).vc
	_, err := vc.CreateVoucher(adminCtx(t), platformVoucher("", promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP))
	if code(err) != codes.InvalidArgument {
		t.Fatalf("got %v, want InvalidArgument", code(err))
	}
	rec.requireNone(t, "empty code")
}
