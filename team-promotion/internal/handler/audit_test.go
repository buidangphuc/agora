package handler_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/service"
	"github.com/buidangphuc/team-promotion/internal/upstream"
)

// recordingAudit is the service.AuditWriter fake: records events, optionally fails.
type recordingAudit struct {
	mu     sync.Mutex
	events []upstream.AuditEvent
	err    error
}

func (r *recordingAudit) WriteEvent(_ context.Context, ev upstream.AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return r.err
}

func (r *recordingAudit) got() []upstream.AuditEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]upstream.AuditEvent(nil), r.events...)
}

func (r *recordingAudit) requireNone(t *testing.T, what string) {
	t.Helper()
	if evs := r.got(); len(evs) != 0 {
		t.Fatalf("%s: wrote %d audit events, want 0: %+v", what, len(evs), evs)
	}
}

var (
	adminCtx  = func() context.Context { return authCtx("admin-1", "listing.write", "admin") }
	sellerCtx = func() context.Context { return authCtx("seller-1", "listing.write") }
	buyerCtx  = func() context.Context { return authCtx("buyer-1", "listing.read") }
	anonCtx   = func() context.Context { return principalCtx("anonymous", "anonymous") }
)

func platformVoucher(code string, scope promotionv1.VoucherScope) *promotionv1.CreateVoucherRequest {
	return &promotionv1.CreateVoucherRequest{
		Code: code, Scope: scope, Quota: 5, DiscountValue: 10,
		DiscountType: promotionv1.DiscountType_DISCOUNT_TYPE_PERCENT,
	}
}

func owned() ownedListings { return ownedListings{"l1": "seller-1", "l2": "seller-2"} }

// Scenario: Unspecified scope is audited like platform (and platform itself,
// carrying the verified admin id as actor).
func TestCreateVoucherUnspecifiedScopeAudited(t *testing.T) {
	for name, scope := range map[string]promotionv1.VoucherScope{
		"unspecified": promotionv1.VoucherScope_VOUCHER_SCOPE_UNSPECIFIED,
		"platform":    promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM,
	} {
		t.Run(name, func(t *testing.T) {
			rec := &recordingAudit{}
			vc, _, _, _ := startAllAudited(t, owned(), rec)
			if _, err := vc.CreateVoucher(adminCtx(), platformVoucher("PLAT10", scope)); err != nil {
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
	vc, _, _, _ := startAllAudited(t, owned(), rec)
	platform := promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"seller", sellerCtx(), codes.PermissionDenied},
		{"buyer", buyerCtx(), codes.PermissionDenied},
		{"anonymous", anonCtx(), codes.Unauthenticated},
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
	vc, _, _, _ := startAllAudited(t, owned(), rec)
	_, err := vc.CreateVoucher(adminCtx(), platformVoucher("", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM))
	if code(err) != codes.InvalidArgument {
		t.Fatalf("got %v, want InvalidArgument", code(err))
	}
	rec.requireNone(t, "empty code")
}

// Scenario: Promotion audit upstream not configured / Audit down blocks a platform
// voucher (handler maps Unavailable through unchanged; nothing persisted).
func TestCreateVoucherAuditDownUnavailable(t *testing.T) {
	platform := promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM
	for name, audit := range map[string]*recordingAudit{
		"audit failing": {err: errors.New("dial tcp 10.0.0.9:50066: connection refused")},
		"audit timeout": {err: context.DeadlineExceeded},
	} {
		t.Run(name, func(t *testing.T) {
			vc, _, _, _ := startAllAudited(t, owned(), audit)
			_, err := vc.CreateVoucher(adminCtx(), platformVoucher("PLAT-DOWN", platform))
			if code(err) != codes.Unavailable {
				t.Fatalf("got %v, want Unavailable (err=%v)", code(err), err)
			}
			if _, err := vc.GetVoucher(adminCtx(), &promotionv1.GetVoucherRequest{Code: "PLAT-DOWN"}); code(err) != codes.NotFound {
				t.Fatalf("voucher must not exist, GetVoucher = %v", code(err))
			}
		})
	}
	// Writer not wired at all (UPSTREAM_AUDIT_ADDR unset).
	vc, _, _, _ := startAllAudited(t, owned(), nil)
	if _, err := vc.CreateVoucher(adminCtx(), platformVoucher("PLAT-NIL", platform)); code(err) != codes.Unavailable {
		t.Fatalf("nil audit: got %v, want Unavailable", code(err))
	}
	if _, err := vc.GetVoucher(adminCtx(), &promotionv1.GetVoucherRequest{Code: "PLAT-NIL"}); code(err) != codes.NotFound {
		t.Fatalf("voucher must not exist, GetVoucher = %v", code(err))
	}
}

// Scenario: Seller cannot create a platform voucher with audit unconfigured.
func TestCreateVoucherPlatformSellerDeniedNilAudit(t *testing.T) {
	vc, _, _, _ := startAllAudited(t, owned(), nil)
	_, err := vc.CreateVoucher(sellerCtx(), platformVoucher("SP", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM))
	if code(err) != codes.PermissionDenied {
		t.Fatalf("got %v, want PermissionDenied (not Unavailable)", code(err))
	}
}

// Scenario: Admin creates a flash sale with an audit event (handler half: the
// verified admin id is the actor, the response id is the target).
func TestCreateCampaignAdminAudited(t *testing.T) {
	rec := &recordingAudit{}
	_, fc, _, _ := startAllAudited(t, owned(), rec)
	resp, err := fc.CreateCampaign(adminCtx(), &promotionv1.CreateCampaignRequest{ListingId: "l2", SalePrice: 1000, StockCap: 5})
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
	_, fc, _, _ := startAllAudited(t, owned(), rec)
	if _, err := fc.CreateCampaign(sellerCtx(), &promotionv1.CreateCampaignRequest{ListingId: "l1", SalePrice: 1000, StockCap: 5}); err != nil {
		t.Fatalf("seller campaign: %v", err)
	}
	rec.requireNone(t, "seller flash sale")
}

// Scenario: Audit down blocks an admin flash sale.
func TestCreateCampaignAuditDownUnavailable(t *testing.T) {
	for name, audit := range map[string]service.AuditWriter{"failing": &recordingAudit{err: errors.New("down")}, "nil": nil} {
		t.Run(name, func(t *testing.T) {
			_, fc, _, _ := startAllAudited(t, owned(), audit)
			_, err := fc.CreateCampaign(adminCtx(), &promotionv1.CreateCampaignRequest{ListingId: "l1", SalePrice: 1000, StockCap: 5})
			if code(err) != codes.Unavailable {
				t.Fatalf("got %v, want Unavailable", code(err))
			}
			got, err := fc.GetActiveFlashSale(adminCtx(), &promotionv1.GetActiveFlashSaleRequest{ListingId: "l1"})
			if err != nil || got.GetActive() {
				t.Fatalf("no sale may exist: active=%v err=%v", got.GetActive(), err)
			}
		})
	}
}

// Scenario: Invalid campaign writes no event.
func TestCreateCampaignInvalidWritesNoAudit(t *testing.T) {
	rec := &recordingAudit{}
	_, fc, _, _ := startAllAudited(t, owned(), rec)
	_, err := fc.CreateCampaign(adminCtx(), &promotionv1.CreateCampaignRequest{ListingId: "", SalePrice: 1000, StockCap: 5})
	if code(err) != codes.InvalidArgument {
		t.Fatalf("got %v, want InvalidArgument", code(err))
	}
	rec.requireNone(t, "empty listing_id")
}

// Scenario: Refused callers write no flash-sale event.
func TestCreateCampaignDeniedWritesNoAudit(t *testing.T) {
	rec := &recordingAudit{}
	_, fc, _, _ := startAllAudited(t, owned(), rec)
	for _, tc := range []struct {
		name    string
		ctx     context.Context
		listing string
		want    codes.Code
	}{
		{"buyer", buyerCtx(), "l1", codes.PermissionDenied},
		{"anonymous", anonCtx(), "l1", codes.Unauthenticated},
		{"seller foreign listing", sellerCtx(), "l2", codes.PermissionDenied},
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
	_, _, _, ads := startAllAudited(t, owned(), rec)
	resp, err := ads.CreateAdCampaign(adminCtx(), &promotionv1.CreateAdCampaignRequest{ListingId: "l2", Bid: 100, Budget: 5000})
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
	_, _, _, ads := startAllAudited(t, owned(), rec)
	if _, err := ads.CreateAdCampaign(sellerCtx(), &promotionv1.CreateAdCampaignRequest{ListingId: "l1", Bid: 10, Budget: 100}); err != nil {
		t.Fatalf("seller ad campaign: %v", err)
	}
	rec.requireNone(t, "seller ad campaign")
}

// Scenario: Audit down blocks an admin ad campaign.
func TestCreateAdCampaignAuditDownUnavailable(t *testing.T) {
	for name, audit := range map[string]service.AuditWriter{"failing": &recordingAudit{err: errors.New("down")}, "nil": nil} {
		t.Run(name, func(t *testing.T) {
			_, _, _, ads := startAllAudited(t, owned(), audit)
			_, err := ads.CreateAdCampaign(adminCtx(), &promotionv1.CreateAdCampaignRequest{ListingId: "l1", Bid: 10, Budget: 100})
			if st, _ := status.FromError(err); st.Code() != codes.Unavailable {
				t.Fatalf("got %v, want Unavailable", st.Code())
			}
			slots, err := ads.ListSponsoredSlots(anonCtx(), &promotionv1.ListSponsoredSlotsRequest{})
			if err != nil || len(slots.GetListingIds()) != 0 {
				t.Fatalf("no campaign may exist: slots=%v err=%v", slots.GetListingIds(), err)
			}
		})
	}
}

// Scenario: Invalid campaign writes no event (ad campaign budget above the cap).
func TestCreateAdCampaignInvalidWritesNoAudit(t *testing.T) {
	rec := &recordingAudit{}
	_, _, _, ads := startAllAudited(t, owned(), rec)
	for name, req := range map[string]*promotionv1.CreateAdCampaignRequest{
		"empty listing":    {ListingId: "", Bid: 10, Budget: 100},
		"budget above cap": {ListingId: "l1", Bid: 10, Budget: 5001},
	} {
		if _, err := ads.CreateAdCampaign(adminCtx(), req); code(err) != codes.InvalidArgument {
			t.Errorf("%s: got %v, want InvalidArgument", name, code(err))
		}
	}
	rec.requireNone(t, "invalid ad campaigns")
}

// Scenario: Refused callers write no ad-campaign event.
func TestCreateAdCampaignDeniedWritesNoAudit(t *testing.T) {
	rec := &recordingAudit{}
	_, _, _, ads := startAllAudited(t, owned(), rec)
	for _, tc := range []struct {
		name    string
		ctx     context.Context
		listing string
	}{
		{"buyer without listing.write", buyerCtx(), "l1"},
		{"seller foreign listing", sellerCtx(), "l2"},
	} {
		_, err := ads.CreateAdCampaign(tc.ctx, &promotionv1.CreateAdCampaignRequest{ListingId: tc.listing, Bid: 10, Budget: 100})
		if code(err) != codes.PermissionDenied {
			t.Errorf("%s: got %v, want PermissionDenied", tc.name, code(err))
		}
	}
	rec.requireNone(t, "refused callers")
}

// Scenario: Seller paths unaffected with audit unconfigured.
func TestSellerPathsIgnoreAuditAvailability(t *testing.T) {
	vc, fc, _, ads := startAllAudited(t, owned(), nil)
	if _, err := vc.CreateVoucher(sellerCtx(), platformVoucher("SHOP-NIL", promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP)); err != nil {
		t.Errorf("shop voucher: %v", err)
	}
	if _, err := fc.CreateCampaign(sellerCtx(), &promotionv1.CreateCampaignRequest{ListingId: "l1", SalePrice: 1000, StockCap: 5}); err != nil {
		t.Errorf("flash sale: %v", err)
	}
	if _, err := ads.CreateAdCampaign(sellerCtx(), &promotionv1.CreateAdCampaignRequest{ListingId: "l1", Bid: 10, Budget: 100}); err != nil {
		t.Errorf("ad campaign: %v", err)
	}
}

// Scenario: Shop voucher audit: admin shop voucher is audited by the handler with
// the admin as actor.
func TestCreateVoucherShopAdminAudited(t *testing.T) {
	rec := &recordingAudit{}
	vc, _, _, _ := startAllAudited(t, owned(), rec)
	if _, err := vc.CreateVoucher(adminCtx(), platformVoucher("SHOP-ADM10", promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP)); err != nil {
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
	for name, audit := range map[string]service.AuditWriter{"failing": &recordingAudit{err: errors.New("down")}, "nil": nil} {
		t.Run(name, func(t *testing.T) {
			vc, _, _, _ := startAllAudited(t, owned(), audit)
			_, err := vc.CreateVoucher(adminCtx(), platformVoucher("SHOP-DOWN", promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP))
			if code(err) != codes.Unavailable {
				t.Fatalf("got %v, want Unavailable (err=%v)", code(err), err)
			}
			if _, err := vc.GetVoucher(adminCtx(), &promotionv1.GetVoucherRequest{Code: "SHOP-DOWN"}); code(err) != codes.NotFound {
				t.Fatalf("voucher must not exist, GetVoucher = %v", code(err))
			}
		})
	}
}

// Scenario: Shop voucher audit: seller path needs no audit writer.
func TestCreateVoucherShopSellerWritesNoAudit(t *testing.T) {
	shop := promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP
	rec := &recordingAudit{err: errors.New("audit down")}
	vc, _, _, _ := startAllAudited(t, owned(), rec)
	if _, err := vc.CreateVoucher(sellerCtx(), platformVoucher("SHOP-SELL10", shop)); err != nil {
		t.Fatalf("seller shop voucher: %v", err)
	}
	rec.requireNone(t, "seller shop voucher")
	vc, _, _, _ = startAllAudited(t, owned(), nil)
	if _, err := vc.CreateVoucher(sellerCtx(), platformVoucher("SHOP-SELL11", shop)); err != nil {
		t.Fatalf("seller shop voucher with nil audit: %v", err)
	}
}

// Scenario: Shop voucher audit: refused callers write nothing.
func TestCreateVoucherShopRefusedWritesNoAudit(t *testing.T) {
	rec := &recordingAudit{}
	vc, _, _, _ := startAllAudited(t, owned(), rec)
	shop := promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"buyer", buyerCtx(), codes.PermissionDenied},
		{"anonymous", anonCtx(), codes.Unauthenticated},
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
	vc, _, _, _ := startAllAudited(t, owned(), rec)
	_, err := vc.CreateVoucher(adminCtx(), platformVoucher("", promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP))
	if code(err) != codes.InvalidArgument {
		t.Fatalf("got %v, want InvalidArgument", code(err))
	}
	rec.requireNone(t, "empty code")
}
