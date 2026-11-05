package rpc_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
)

func bareAdminCtx(t *testing.T) context.Context {
	t.Helper()
	return authCtx(t, "admin-2", "listing.read", "listing.write", "admin", "order.admin")
}

func adminMarkerOnlyCtx(t *testing.T) context.Context {
	t.Helper()
	return authCtx(t, "admin-3", "admin")
}

func promotionAdminOnlyCtx(t *testing.T) context.Context {
	t.Helper()
	return authCtx(t, "ops-1", "listing.write", "promotion.admin")
}

type adminOnlyCall struct {
	name string
	call func(ctx context.Context, r *rig, voucherID, campaignID string) error
}

func adminOnlyCalls() []adminOnlyCall {
	return []adminOnlyCall{
		{"DeactivateVoucher", func(ctx context.Context, r *rig, v, _ string) error {
			_, err := r.vc.DeactivateVoucher(ctx, &promotionv1.DeactivateVoucherRequest{VoucherId: v})
			return err
		}},
		{"UpdateVoucher", func(ctx context.Context, r *rig, v, _ string) error {
			_, err := r.vc.UpdateVoucher(ctx, &promotionv1.UpdateVoucherRequest{VoucherId: v, Quota: proto.Int64(9)})
			return err
		}},
		{"ListCampaigns", func(ctx context.Context, r *rig, _, _ string) error {
			_, err := r.fc.ListCampaigns(ctx, &promotionv1.ListCampaignsRequest{})
			return err
		}},
		{"CancelCampaign", func(ctx context.Context, r *rig, _, c string) error {
			_, err := r.fc.CancelCampaign(ctx, &promotionv1.CancelCampaignRequest{CampaignId: c})
			return err
		}},
		{"UpdateCampaign", func(ctx context.Context, r *rig, _, c string) error {
			_, err := r.fc.UpdateCampaign(ctx, &promotionv1.UpdateCampaignRequest{
				CampaignId: c, SalePrice: 40000, StockCap: 5,
				StartsAt: timestamppb.New(time.Now().Add(3 * time.Hour)), EndsAt: timestamppb.New(time.Now().Add(4 * time.Hour)),
			})
			return err
		}},
		{"CreateVoucher PLATFORM", func(ctx context.Context, r *rig, _, _ string) error {
			_, err := r.vc.CreateVoucher(ctx, platformVoucher("PAS-PLAT", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM))
			return err
		}},
		{"CreateVoucher UNSPECIFIED", func(ctx context.Context, r *rig, _, _ string) error {
			_, err := r.vc.CreateVoucher(ctx, platformVoucher("PAS-UNSP", promotionv1.VoucherScope_VOUCHER_SCOPE_UNSPECIFIED))
			return err
		}},
	}
}

func seedAdminFixtures(t *testing.T) (*campaignRig, string, string) {
	t.Helper()
	c := newCampaignRig(t)
	v, err := c.vc.CreateVoucher(adminCtx(t), platformVoucher("PAS-SEED", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM))
	if err != nil {
		t.Fatalf("seed voucher: %v", err)
	}
	id := c.seed(t, "l1", time.Hour, 2*time.Hour)
	c.rec.mu.Lock()
	c.rec.events = nil
	c.rec.mu.Unlock()
	return c, v.GetVoucher().GetId(), id
}

func TestAdminOnlyRPCsNeedAdminAndPromotionAdmin(t *testing.T) {
	refused := []struct {
		name string
		ctx  func(*testing.T) context.Context
		want codes.Code
	}{
		{"admin without promotion.admin", bareAdminCtx, codes.PermissionDenied},
		{"admin marker only", adminMarkerOnlyCtx, codes.PermissionDenied},
		{"promotion.admin without admin", promotionAdminOnlyCtx, codes.PermissionDenied},
		{"seller", sellerCtx, codes.PermissionDenied},
		{"buyer", buyerCtx, codes.PermissionDenied},
		{"service", orderSvcCtx, codes.PermissionDenied},
		{"anonymous", anonCtx, codes.Unauthenticated},
	}
	for _, call := range adminOnlyCalls() {
		t.Run(call.name, func(t *testing.T) {
			c, voucherID, campaignID := seedAdminFixtures(t)
			for _, who := range refused {
				if err := call.call(who.ctx(t), c.rig, voucherID, campaignID); code(err) != who.want {
					t.Errorf("%s: got %v, want %v (err=%v)", who.name, code(err), who.want, err)
				}
			}
			c.rec.requireNone(t, call.name+" refused callers")
			v, err := c.vc.GetVoucher(t.Context(), &promotionv1.GetVoucherRequest{Code: "PAS-SEED"})
			if err != nil || !v.GetVoucher().GetActive() || v.GetVoucher().GetQuota() != 5 {
				t.Fatalf("voucher changed by a refused call: %+v %v", v.GetVoucher(), err)
			}
			if st := c.statusOf(t, campaignID); st != promotionv1.CampaignStatus_CAMPAIGN_STATUS_SCHEDULED {
				t.Fatalf("campaign changed by a refused call: %v", st)
			}
			for _, vc := range []string{"PAS-PLAT", "PAS-UNSP"} {
				if _, err := c.vc.GetVoucher(t.Context(), &promotionv1.GetVoucherRequest{Code: vc}); code(err) != codes.NotFound {
					t.Fatalf("voucher %s created by a refused call: %v", vc, err)
				}
			}
			if err := call.call(adminCtx(t), c.rig, voucherID, campaignID); err != nil {
				t.Fatalf("admin with promotion.admin: %v", err)
			}
		})
	}
}

func TestShopVoucherBareAdminTakesSellerPath(t *testing.T) {
	rec := &recordingAudit{}
	r := startRig(t, rigOpts{audit: rec})
	resp, err := r.vc.CreateVoucher(bareAdminCtx(t), platformVoucher("PAS-SHOP", promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP))
	if err != nil {
		t.Fatalf("bare admin shop voucher: %v", err)
	}
	if got := resp.GetVoucher().GetSellerId(); got != "admin-2" {
		t.Fatalf("seller_id = %q, want the caller's id", got)
	}
	rec.requireNone(t, "bare admin shop voucher")
}

func TestShopVoucherBareAdminIgnoresAuditOutage(t *testing.T) {
	rec := &recordingAudit{err: errors.New("audit down")}
	r := startRig(t, rigOpts{audit: rec})
	if _, err := r.vc.CreateVoucher(bareAdminCtx(t), platformVoucher("PAS-SHOP-DOWN", promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP)); err != nil {
		t.Fatalf("bare admin shop voucher with audit down: %v", err)
	}
	rec.requireNone(t, "bare admin shop voucher with audit down")
}

func TestShopVoucherAdminMarkerOnlyDenied(t *testing.T) {
	rec := &recordingAudit{}
	r := startRig(t, rigOpts{audit: rec})
	_, err := r.vc.CreateVoucher(adminMarkerOnlyCtx(t), platformVoucher("PAS-SHOP-X", promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP))
	if code(err) != codes.PermissionDenied {
		t.Fatalf("got %v, want PermissionDenied (err=%v)", code(err), err)
	}
	rec.requireNone(t, "admin marker only shop voucher")
	if _, err := r.vc.GetVoucher(t.Context(), &promotionv1.GetVoucherRequest{Code: "PAS-SHOP-X"}); code(err) != codes.NotFound {
		t.Fatalf("voucher must not exist: %v", err)
	}
}

func TestShopVoucherPromotionAdminAudited(t *testing.T) {
	rec := &recordingAudit{}
	r := startRig(t, rigOpts{audit: rec})
	if _, err := r.vc.CreateVoucher(promotionAdminOnlyCtx(t), platformVoucher("PAS-SHOP-PA", promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP)); err != nil {
		t.Fatalf("promotion.admin shop voucher: %v", err)
	}
	evs := rec.got()
	if len(evs) != 1 || evs[0].Action != "voucher.admin_create" || evs[0].ActorID != "ops-1" {
		t.Fatalf("want one voucher.admin_create by ops-1, got %+v", evs)
	}
}

func TestCreateCampaignBareAdminTakesSellerPath(t *testing.T) {
	rec := &recordingAudit{}
	r := startRig(t, rigOpts{audit: rec})
	_, err := r.fc.CreateCampaign(bareAdminCtx(t), newCreateReq("l1"))
	if code(err) != codes.PermissionDenied {
		t.Fatalf("foreign listing: got %v, want PermissionDenied (err=%v)", code(err), err)
	}
	rec.requireNone(t, "bare admin foreign campaign")
	resp, err := r.fc.ListCampaigns(adminCtx(t), &promotionv1.ListCampaignsRequest{})
	if err != nil || len(resp.GetCampaigns()) != 0 {
		t.Fatalf("no campaign may exist: %v %v", resp.GetCampaigns(), err)
	}
}

func TestCreateCampaignBareAdminOwnListingNeedsNoIdempotencyKey(t *testing.T) {
	rec := &recordingAudit{}
	r := startRig(t, rigOpts{audit: rec, listings: ownedListings{"l9": "admin-2"}})
	req := newCreateReq("l9")
	req.IdempotencyKey = ""
	if _, err := r.fc.CreateCampaign(bareAdminCtx(t), req); err != nil {
		t.Fatalf("bare admin own listing: %v", err)
	}
	rec.requireNone(t, "bare admin own-listing campaign")
}

func TestCreateCampaignAdminMarkerOnlyDenied(t *testing.T) {
	rec := &recordingAudit{}
	r := startRig(t, rigOpts{audit: rec})
	if _, err := r.fc.CreateCampaign(adminMarkerOnlyCtx(t), newCreateReq("l1")); code(err) != codes.PermissionDenied {
		t.Fatalf("got %v, want PermissionDenied (err=%v)", code(err), err)
	}
	rec.requireNone(t, "admin marker only campaign")
}

func TestCreateCampaignPromotionAdminAudited(t *testing.T) {
	rec := &recordingAudit{}
	r := startRig(t, rigOpts{audit: rec})
	if _, err := r.fc.CreateCampaign(promotionAdminOnlyCtx(t), newCreateReq("l1")); err != nil {
		t.Fatalf("promotion.admin campaign: %v", err)
	}
	evs := rec.got()
	if len(evs) != 1 || evs[0].Action != "flash_sale.admin_create" || evs[0].ActorID != "ops-1" {
		t.Fatalf("want one flash_sale.admin_create by ops-1, got %+v", evs)
	}
}

func TestCreateAdCampaignBareAdminTakesSellerPath(t *testing.T) {
	rec := &recordingAudit{}
	r := startRig(t, rigOpts{audit: rec})
	_, err := r.ads.CreateAdCampaign(bareAdminCtx(t), &promotionv1.CreateAdCampaignRequest{ListingId: "l1", Bid: 10, Budget: 100})
	if code(err) != codes.PermissionDenied {
		t.Fatalf("foreign listing: got %v, want PermissionDenied (err=%v)", code(err), err)
	}
	rec.requireNone(t, "bare admin foreign ad campaign")
}

func TestCreateAdCampaignPromotionAdminAudited(t *testing.T) {
	rec := &recordingAudit{}
	r := startRig(t, rigOpts{audit: rec})
	if _, err := r.ads.CreateAdCampaign(promotionAdminOnlyCtx(t), &promotionv1.CreateAdCampaignRequest{ListingId: "l1", Bid: 10, Budget: 100}); err != nil {
		t.Fatalf("promotion.admin ad campaign: %v", err)
	}
	evs := rec.got()
	if len(evs) != 1 || evs[0].Action != "ad_campaign.admin_create" || evs[0].ActorID != "ops-1" {
		t.Fatalf("want one ad_campaign.admin_create by ops-1, got %+v", evs)
	}
}
