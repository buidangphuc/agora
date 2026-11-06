package rpc_test

import (
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/store"
)

func seedOwned(t *testing.T, r *rig, code, owner string, mod func(*store.Voucher)) store.Voucher {
	t.Helper()
	now := time.Now()
	v := store.Voucher{
		Code: code, Scope: 2, DiscountType: 2, DiscountValue: 20000, MinSpend: 100000, Quota: 1, PerUserLimit: 1,
		OwnerUserID: owner, Kind: store.VoucherKindNewUser,
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(24 * time.Hour),
	}
	if mod != nil {
		mod(&v)
	}
	out, err := r.vouchers.Create(t.Context(), v, voucherRow)
	if err != nil {
		t.Fatalf("seed %s: %v", code, err)
	}
	return out
}

func codesOfProto(vs []*promotionv1.Voucher) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.GetCode())
	}
	return out
}

func TestListMyVouchersOwnerOnly(t *testing.T) {
	r := startRig(t, rigOpts{})
	seedOwned(t, r, "NEWU-A", "user-a", nil)
	seedOwned(t, r, "NEWU-B", "user-b", nil)

	resp, err := r.vc.ListMyVouchers(authCtx(t, "user-a"), &promotionv1.ListMyVouchersRequest{})
	if err != nil {
		t.Fatalf("ListMyVouchers: %v", err)
	}
	got := resp.GetVouchers()
	if len(got) != 1 || got[0].GetCode() != "NEWU-A" {
		t.Fatalf("A sees %v, want [NEWU-A]", codesOfProto(got))
	}
	v := got[0]
	if v.GetKind() != promotionv1.VoucherKind_VOUCHER_KIND_NEW_USER || v.GetOwnerUserId() != "user-a" || v.GetPerUserLimit() != 1 || v.GetQuota() != 1 || v.GetDiscountValue() != 20000 || v.GetMinSpend() != 100000 {
		t.Fatalf("voucher = %v", v)
	}
	for _, x := range got {
		if x.GetOwnerUserId() == "user-b" {
			t.Fatalf("B's voucher leaked to A: %v", x)
		}
	}
}

func TestListMyVouchersNoScope(t *testing.T) {
	r := startRig(t, rigOpts{})
	seedOwned(t, r, "NEWU-A", "user-a", nil)
	for name, ctx := range map[string]func() (int, error){
		"no scopes": func() (int, error) {
			resp, err := r.vc.ListMyVouchers(authCtx(t, "user-a"), &promotionv1.ListMyVouchersRequest{})
			return len(resp.GetVouchers()), err
		},
		"unrelated scope": func() (int, error) {
			resp, err := r.vc.ListMyVouchers(authCtx(t, "user-a", "listing.read"), &promotionv1.ListMyVouchersRequest{})
			return len(resp.GetVouchers()), err
		},
	} {
		n, err := ctx()
		if err != nil || n != 1 {
			t.Errorf("%s: n=%d err=%v, want 1 voucher and OK", name, n, err)
		}
	}
	if _, err := r.vc.ListMyVouchers(anonCtx(t), &promotionv1.ListMyVouchersRequest{}); code(err) != codes.Unauthenticated {
		t.Errorf("anonymous: got %v, want Unauthenticated", code(err))
	}
	if _, err := r.vc.ListMyVouchers(t.Context(), &promotionv1.ListMyVouchersRequest{}); code(err) != codes.Unauthenticated {
		t.Errorf("no principal: got %v, want Unauthenticated", code(err))
	}
}

func TestListMyVouchersFilters(t *testing.T) {
	r := startRig(t, rigOpts{})
	now := time.Now()
	used := seedOwned(t, r, "USED", "user-a", nil)
	if err := r.vouchers.IncrementUsed(t.Context(), used.ID, 1); err != nil {
		t.Fatal(err)
	}
	seedOwned(t, r, "EXPIRED", "user-a", func(v *store.Voucher) { v.EndsAt = now })
	off := seedOwned(t, r, "OFF", "user-a", nil)
	if _, err := r.vouchers.Deactivate(t.Context(), off.ID, now); err != nil {
		t.Fatal(err)
	}
	seedOwned(t, r, "FUTURE", "user-a", func(v *store.Voucher) { v.StartsAt = now.Add(time.Hour); v.EndsAt = now.Add(48 * time.Hour) })
	seedOwned(t, r, "LIVE", "user-a", func(v *store.Voucher) { v.EndsAt = now.Add(time.Minute) })
	held := seedOwned(t, r, "HELD", "user-a", func(v *store.Voucher) { v.EndsAt = now.Add(2 * time.Minute) })
	if _, _, err := r.reservations.Create(t.Context(), store.Reservation{
		ReservationID: "hold-1", VoucherID: held.ID, BuyerID: "user-a", DiscountAmount: 20000,
		Status: store.ReservationReserved, ExpiresAt: now.Add(15 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	resp, err := r.vc.ListMyVouchers(authCtx(t, "user-a"), &promotionv1.ListMyVouchersRequest{})
	if err != nil {
		t.Fatalf("ListMyVouchers: %v", err)
	}
	got := fmt.Sprint(codesOfProto(resp.GetVouchers()))
	if got != "[LIVE HELD]" {
		t.Fatalf("visible = %s, want [LIVE HELD] (soonest expiry first; used, expired, inactive and not-yet-started hidden; live hold stays)", got)
	}
}

func TestListMyVouchersEmpty(t *testing.T) {
	r := startRig(t, rigOpts{})
	seedOwned(t, r, "NEWU-A", "user-a", nil)
	resp, err := r.vc.ListMyVouchers(authCtx(t, "user-c"), &promotionv1.ListMyVouchersRequest{})
	if err != nil {
		t.Fatalf("ListMyVouchers: %v", err)
	}
	if n := len(resp.GetVouchers()); n != 0 {
		t.Fatalf("got %d vouchers, want 0", n)
	}
}

func TestGetVoucherOwnerOnly(t *testing.T) {
	r := startRig(t, rigOpts{})
	seedOwned(t, r, "NEWU-A", "user-a", nil)
	if _, err := r.vc.CreateVoucher(adminCtx(t), platformVoucher("PUBLIC1", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM)); err != nil {
		t.Fatalf("seed public: %v", err)
	}
	req := &promotionv1.GetVoucherRequest{Code: "NEWU-A"}

	if _, err := r.vc.GetVoucher(authCtx(t, "user-b"), req); code(err) != codes.NotFound {
		t.Errorf("non-owner: got %v, want NotFound", code(err))
	}
	if _, err := r.vc.GetVoucher(anonCtx(t), req); code(err) != codes.NotFound {
		t.Errorf("anonymous: got %v, want NotFound", code(err))
	}
	resp, err := r.vc.GetVoucher(authCtx(t, "user-a"), req)
	if err != nil || resp.GetVoucher().GetCode() != "NEWU-A" || resp.GetVoucher().GetOwnerUserId() != "user-a" {
		t.Fatalf("owner: %v err=%v, want the voucher with owner_user_id", resp.GetVoucher(), err)
	}
	pub, err := r.vc.GetVoucher(anonCtx(t), &promotionv1.GetVoucherRequest{Code: "PUBLIC1"})
	if err != nil || pub.GetVoucher().GetOwnerUserId() != "" {
		t.Fatalf("public voucher: %v err=%v, want readable with empty owner", pub.GetVoucher(), err)
	}
}

func TestListVouchersHidesOwnedAndOwnerIDs(t *testing.T) {
	r := startRig(t, rigOpts{})
	for i := 0; i < 3; i++ {
		seedOwned(t, r, fmt.Sprintf("NEWU-%d", i), fmt.Sprintf("user-%d", i), nil)
	}
	for _, c := range []string{"PUB1", "PUB2"} {
		if _, err := r.vc.CreateVoucher(adminCtx(t), platformVoucher(c, promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM)); err != nil {
			t.Fatalf("seed %s: %v", c, err)
		}
	}
	resp, err := r.vc.ListVouchers(anonCtx(t), &promotionv1.ListVouchersRequest{})
	if err != nil {
		t.Fatalf("ListVouchers: %v", err)
	}
	if len(resp.GetVouchers()) != 2 {
		t.Fatalf("public list = %v, want the 2 public vouchers", codesOfProto(resp.GetVouchers()))
	}
	for _, v := range resp.GetVouchers() {
		if v.GetOwnerUserId() != "" {
			t.Fatalf("public list leaked owner_user_id on %s", v.GetCode())
		}
	}
}

func TestNoSelfIssue(t *testing.T) {
	r := startRig(t, rigOpts{})
	resp, err := r.vc.ListMyVouchers(authCtx(t, "user-never-registered"), &promotionv1.ListMyVouchersRequest{})
	if err != nil || len(resp.GetVouchers()) != 0 {
		t.Fatalf("ListMyVouchers = %v err=%v, want zero vouchers", resp.GetVouchers(), err)
	}
	buyer := authCtx(t, "user-never-registered", "listing.read")
	for _, scope := range []promotionv1.VoucherScope{
		promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP,
		promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM,
		promotionv1.VoucherScope_VOUCHER_SCOPE_UNSPECIFIED,
	} {
		req := platformVoucher("SELF-"+scope.String(), scope)
		req.PerUserLimit = 1
		req.NewUserOnly = true
		if _, err := r.vc.CreateVoucher(buyer, req); code(err) != codes.PermissionDenied {
			t.Errorf("buyer CreateVoucher scope %v: got %v, want PermissionDenied", scope, code(err))
		}
	}
	after, err := r.vc.ListMyVouchers(buyer, &promotionv1.ListMyVouchersRequest{})
	if err != nil || len(after.GetVouchers()) != 0 {
		t.Fatalf("after attempts: %v err=%v, want zero vouchers", after.GetVouchers(), err)
	}
}
