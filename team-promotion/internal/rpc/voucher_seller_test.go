package rpc_test

import (
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-promotion/generated/platform/common/v1"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/store"
)

type sellerVoucherRig struct {
	*rig
	rec *recordingAudit
}

func newSellerVoucherRig(t *testing.T) *sellerVoucherRig {
	t.Helper()
	rec := &recordingAudit{}
	return &sellerVoucherRig{rig: startRig(t, rigOpts{audit: rec}), rec: rec}
}

func (r *sellerVoucherRig) seed(t *testing.T, code, seller string, scope int32, ends time.Time) store.Voucher {
	t.Helper()
	now := time.Now()
	v, err := r.vouchers.Create(t.Context(), store.Voucher{
		Code: code, Scope: scope, SellerID: seller, DiscountType: 1, DiscountValue: 10,
		MinSpend: 1000, MaxDiscount: 500, Quota: 7,
		StartsAt: now.Add(-2 * time.Hour), EndsAt: ends,
	}, voucherRow)
	if err != nil {
		t.Fatalf("seed %s: %v", code, err)
	}
	return v
}

func TestListMyShopVouchersOwn_SellerSeesOwnVouchersOfEveryState(t *testing.T) {
	r := newSellerVoucherRig(t)
	future := time.Now().Add(24 * time.Hour)
	r.seed(t, "S1-LIVE", "seller-1", 1, future)
	off := r.seed(t, "S1-OFF", "seller-1", 1, future)
	r.seed(t, "S1-OLD", "seller-1", 1, time.Now().Add(-time.Hour))
	r.seed(t, "S2-LIVE", "seller-2", 1, future)
	r.seed(t, "PLAT", "", 2, future)
	r.seed(t, "PLAT-UNDER-S1", "seller-1", 2, future)
	if _, err := r.vouchers.Deactivate(t.Context(), off.ID, time.Now()); err != nil {
		t.Fatal(err)
	}

	resp, err := r.vc.ListMyShopVouchers(sellerCtx(t), &promotionv1.ListMyShopVouchersRequest{})
	if err != nil {
		t.Fatalf("ListMyShopVouchers: %v", err)
	}
	got := map[string]*promotionv1.Voucher{}
	for _, v := range resp.GetVouchers() {
		got[v.GetCode()] = v
	}
	if len(resp.GetVouchers()) != 3 || got["S1-LIVE"] == nil || got["S1-OFF"] == nil || got["S1-OLD"] == nil {
		t.Fatalf("vouchers = %v, want exactly S1-LIVE S1-OFF S1-OLD", got)
	}
	if got["S1-LIVE"].GetQuota() != 7 || !got["S1-LIVE"].GetActive() {
		t.Errorf("S1-LIVE = %v, want quota 7 active", got["S1-LIVE"])
	}
	if got["S1-OFF"].GetActive() || got["S1-OFF"].GetDeactivatedAt() == nil {
		t.Errorf("S1-OFF = %v, want inactive with deactivated_at", got["S1-OFF"])
	}
	if resp.GetPage().GetNextCursor() != "" {
		t.Errorf("next cursor = %q, want empty", resp.GetPage().GetNextCursor())
	}
	r.rec.requireNone(t, "ListMyShopVouchers")
}

func TestListMyShopVouchersEmpty(t *testing.T) {
	r := newSellerVoucherRig(t)
	r.seed(t, "S2-LIVE", "seller-2", 1, time.Now().Add(time.Hour))
	resp, err := r.vc.ListMyShopVouchers(sellerCtx(t), &promotionv1.ListMyShopVouchersRequest{})
	if err != nil {
		t.Fatalf("ListMyShopVouchers: %v", err)
	}
	if len(resp.GetVouchers()) != 0 || resp.GetPage().GetNextCursor() != "" {
		t.Fatalf("resp = %v, want zero vouchers and empty cursor", resp)
	}
}

func TestListMyShopVouchersAuthz_BuyerWithoutListingWriteIsDenied(t *testing.T) {
	r := newSellerVoucherRig(t)
	r.seed(t, "BUYER-1", "buyer-1", 1, time.Now().Add(time.Hour))
	r.seed(t, "ADMIN-1", "admin-1", 1, time.Now().Add(time.Hour))
	for _, tc := range []struct {
		name string
		ctx  func() (*promotionv1.ListMyShopVouchersResponse, error)
		want codes.Code
	}{
		{"buyer", func() (*promotionv1.ListMyShopVouchersResponse, error) {
			return r.vc.ListMyShopVouchers(buyerCtx(t), &promotionv1.ListMyShopVouchersRequest{})
		}, codes.PermissionDenied},
		{"admin without listing.write", func() (*promotionv1.ListMyShopVouchersResponse, error) {
			return r.vc.ListMyShopVouchers(authCtx(t, "admin-1", "admin", "promotion.admin"), &promotionv1.ListMyShopVouchersRequest{})
		}, codes.PermissionDenied},
		{"anonymous", func() (*promotionv1.ListMyShopVouchersResponse, error) {
			return r.vc.ListMyShopVouchers(anonCtx(t), &promotionv1.ListMyShopVouchersRequest{})
		}, codes.Unauthenticated},
	} {
		resp, err := tc.ctx()
		if code(err) != tc.want {
			t.Errorf("%s: code = %v, want %v", tc.name, code(err), tc.want)
		}
		if len(resp.GetVouchers()) != 0 {
			t.Errorf("%s: leaked %d vouchers", tc.name, len(resp.GetVouchers()))
		}
	}
}

func TestListMyShopVouchersPageCap_PageSizeAboveTheMaximumIsCapped(t *testing.T) {
	r := newSellerVoucherRig(t)
	for i := 0; i < 205; i++ {
		r.seed(t, fmt.Sprintf("S1-%03d", i), "seller-1", 1, time.Now().Add(time.Hour))
	}
	resp, err := r.vc.ListMyShopVouchers(sellerCtx(t), &promotionv1.ListMyShopVouchersRequest{Page: &commonv1.PageRequest{PageSize: 1000}})
	if err != nil {
		t.Fatalf("ListMyShopVouchers: %v", err)
	}
	if n := len(resp.GetVouchers()); n == 0 || n > 200 {
		t.Fatalf("page_size 1000 returned %d, want 1..200", n)
	}
	if resp.GetPage().GetNextCursor() == "" {
		t.Fatal("want a next cursor, 205 vouchers exist")
	}
}

func deactivateMine(r *sellerVoucherRig, t *testing.T, id string) (*promotionv1.DeactivateMyShopVoucherResponse, error) {
	t.Helper()
	return r.vc.DeactivateMyShopVoucher(sellerCtx(t), &promotionv1.DeactivateMyShopVoucherRequest{VoucherId: id})
}

func TestDeactivateMyShopVoucher_SellerDeactivatesTheirVoucher(t *testing.T) {
	r := newSellerVoucherRig(t)
	v := r.seed(t, "S1-LIVE", "seller-1", 1, time.Now().Add(time.Hour))
	resp, err := deactivateMine(r, t, v.ID)
	if err != nil {
		t.Fatalf("DeactivateMyShopVoucher: %v", err)
	}
	got := resp.GetVoucher()
	if got.GetId() != v.ID || got.GetActive() || got.GetDeactivatedAt() == nil || got.GetDeactivatedAt().AsTime().IsZero() {
		t.Fatalf("voucher = %v, want inactive with deactivated_at", got)
	}
	stored, _ := r.vouchers.GetByID(t.Context(), v.ID)
	if stored.Active {
		t.Fatal("voucher still active in the store")
	}
	res, err := r.vc.ValidateAndReserve(orderSvcCtx(t), &promotionv1.ValidateAndReserveRequest{
		ReservationId: "res-1", Code: "S1-LIVE", BuyerId: "buyer-1", CartSubtotal: 10000, SellerId: "seller-1",
	})
	if err != nil || res.GetValid() {
		t.Fatalf("ValidateAndReserve: valid=%v err=%v, want invalid", res.GetValid(), err)
	}
	r.rec.requireNone(t, "DeactivateMyShopVoucher")
}

func TestDeactivateMyShopVoucherIdempotent_DeactivatingTwiceKeepsTheFirstTimestamp(t *testing.T) {
	r := newSellerVoucherRig(t)
	v := r.seed(t, "S1-LIVE", "seller-1", 1, time.Now().Add(time.Hour))
	a, err := deactivateMine(r, t, v.ID)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	b, err := deactivateMine(r, t, v.ID)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if a.GetVoucher().GetDeactivatedAt() == nil || !a.GetVoucher().GetDeactivatedAt().AsTime().Equal(b.GetVoucher().GetDeactivatedAt().AsTime()) {
		t.Fatalf("deactivated_at %v vs %v, want identical", a.GetVoucher().GetDeactivatedAt(), b.GetVoucher().GetDeactivatedAt())
	}
}

func TestDeactivateMyShopVoucherNotOwner_DeactivatingAnotherSellersVoucherIsDeniedLikeAnUnknownId(t *testing.T) {
	r := newSellerVoucherRig(t)
	v := r.seed(t, "S1-LIVE", "seller-1", 1, time.Now().Add(time.Hour))
	other := authCtx(t, "seller-2", "listing.write")
	_, foreign := r.vc.DeactivateMyShopVoucher(other, &promotionv1.DeactivateMyShopVoucherRequest{VoucherId: v.ID})
	_, unknown := r.vc.DeactivateMyShopVoucher(other, &promotionv1.DeactivateMyShopVoucherRequest{VoucherId: "no-such-id"})
	_, empty := r.vc.DeactivateMyShopVoucher(other, &promotionv1.DeactivateMyShopVoucherRequest{})
	for name, err := range map[string]error{"foreign": foreign, "unknown": unknown, "empty": empty} {
		if code(err) != codes.PermissionDenied {
			t.Errorf("%s: code = %v, want PermissionDenied", name, code(err))
		}
	}
	if status.Convert(foreign).Message() != status.Convert(unknown).Message() {
		t.Fatalf("messages differ: %q vs %q", status.Convert(foreign).Message(), status.Convert(unknown).Message())
	}
	stored, _ := r.vouchers.GetByID(t.Context(), v.ID)
	if !stored.Active || !stored.DeactivatedAt.IsZero() {
		t.Fatalf("voucher changed by a denied call: %+v", stored)
	}
}

func TestDeactivateMyShopVoucherPlatformScopeDenied(t *testing.T) {
	r := newSellerVoucherRig(t)
	plat := r.seed(t, "PLAT", "", 2, time.Now().Add(time.Hour))
	under := r.seed(t, "PLAT-S1", "seller-1", 2, time.Now().Add(time.Hour))
	for _, v := range []store.Voucher{plat, under} {
		if _, err := deactivateMine(r, t, v.ID); code(err) != codes.PermissionDenied {
			t.Errorf("%s: code = %v, want PermissionDenied", v.Code, code(err))
		}
		stored, _ := r.vouchers.GetByID(t.Context(), v.ID)
		if !stored.Active {
			t.Errorf("%s was deactivated", v.Code)
		}
	}
}

func TestDeactivateMyShopVoucherAuthz_BuyerWithoutListingWriteIsDenied(t *testing.T) {
	r := newSellerVoucherRig(t)
	v := r.seed(t, "BUYER-1", "buyer-1", 1, time.Now().Add(time.Hour))
	if _, err := r.vc.DeactivateMyShopVoucher(buyerCtx(t), &promotionv1.DeactivateMyShopVoucherRequest{VoucherId: v.ID}); code(err) != codes.PermissionDenied {
		t.Errorf("buyer: code = %v, want PermissionDenied", code(err))
	}
	if _, err := r.vc.DeactivateMyShopVoucher(anonCtx(t), &promotionv1.DeactivateMyShopVoucherRequest{VoucherId: v.ID}); code(err) != codes.Unauthenticated {
		t.Errorf("anonymous: code = %v, want Unauthenticated", code(err))
	}
	stored, _ := r.vouchers.GetByID(t.Context(), v.ID)
	if !stored.Active {
		t.Fatal("voucher deactivated by an unauthorised call")
	}
}

func TestDeactivateMyShopVoucherNoAudit_SellerDeactivationWritesNoAuditEvent(t *testing.T) {
	rec := &recordingAudit{err: status.Error(codes.Unavailable, "team-audit down")}
	r := &sellerVoucherRig{rig: startRig(t, rigOpts{audit: rec}), rec: rec}
	v := r.seed(t, "S1-LIVE", "seller-1", 1, time.Now().Add(time.Hour))
	if _, err := deactivateMine(r, t, v.ID); err != nil {
		t.Fatalf("deactivate with audit down: %v", err)
	}
	rec.requireNone(t, "DeactivateMyShopVoucher")
}

func TestDeactivateMyShopVoucherAdminRegression_AdminDeactivateKeepsItsAdminRules(t *testing.T) {
	r := newSellerVoucherRig(t)
	v := r.seed(t, "S1-LIVE", "seller-1", 1, time.Now().Add(time.Hour))
	if _, err := r.vc.DeactivateVoucher(sellerCtx(t), &promotionv1.DeactivateVoucherRequest{VoucherId: v.ID}); code(err) != codes.PermissionDenied {
		t.Fatalf("seller on admin RPC: code = %v, want PermissionDenied", code(err))
	}
	if _, err := r.vc.DeactivateVoucher(adminCtx(t), &promotionv1.DeactivateVoucherRequest{VoucherId: v.ID}); err != nil {
		t.Fatalf("admin deactivate: %v", err)
	}
	evs := r.rec.got()
	if len(evs) != 1 || evs[0].Action != "voucher.deactivate" || evs[0].TargetID != "S1-LIVE" {
		t.Fatalf("events = %+v, want one voucher.deactivate for S1-LIVE", evs)
	}
}
