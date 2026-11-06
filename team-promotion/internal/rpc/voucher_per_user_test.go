package rpc_test

import (
	"testing"

	"google.golang.org/grpc/codes"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/store"
)

func shopVoucher(code string, perUser int32, newUserOnly bool) *promotionv1.CreateVoucherRequest {
	req := platformVoucher(code, promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP)
	req.PerUserLimit = perUser
	req.NewUserOnly = newUserOnly
	return req
}

func TestCreateAcceptsPerUserFields(t *testing.T) {
	rec := &recordingAudit{}
	r := startRig(t, rigOpts{audit: rec})
	resp, err := r.vc.CreateVoucher(sellerCtx(t), shopVoucher("SHOP-PU", 1, true))
	if err != nil {
		t.Fatalf("CreateVoucher: %v", err)
	}
	v := resp.GetVoucher()
	if v.GetPerUserLimit() != 1 || !v.GetNewUserOnly() || v.GetKind() != promotionv1.VoucherKind_VOUCHER_KIND_STANDARD || v.GetOwnerUserId() != "" {
		t.Fatalf("voucher = per_user_limit %d new_user_only %v kind %v owner %q, want 1 true STANDARD ''", v.GetPerUserLimit(), v.GetNewUserOnly(), v.GetKind(), v.GetOwnerUserId())
	}
	stored, err := r.vouchers.GetByCode(t.Context(), "SHOP-PU")
	if err != nil || stored.PerUserLimit != 1 || !stored.NewUserOnly || stored.Kind != store.VoucherKindStandard || stored.OwnerUserID != "" {
		t.Fatalf("stored = %+v err=%v", stored, err)
	}

	pReq := platformVoucher("PLAT-PU", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM)
	pReq.PerUserLimit = 3
	pReq.NewUserOnly = true
	if _, err := r.vc.CreateVoucher(adminCtx(t), pReq); err != nil {
		t.Fatalf("admin platform create: %v", err)
	}
	evs := rec.got()
	if len(evs) != 1 {
		t.Fatalf("audit events = %d, want 1 (platform create only)", len(evs))
	}
	if evs[0].Metadata["per_user_limit"] != "3" || evs[0].Metadata["new_user_only"] != "true" {
		t.Fatalf("audit metadata = %v, want per_user_limit=3 new_user_only=true", evs[0].Metadata)
	}
}

func TestPerUserLimitBounds(t *testing.T) {
	r := startRig(t, rigOpts{})
	for _, tc := range []struct {
		code string
		n    int32
		want codes.Code
	}{
		{"PU-NEG", -1, codes.InvalidArgument},
		{"PU-1001", 1001, codes.InvalidArgument},
		{"PU-0", 0, codes.OK},
		{"PU-1000", 1000, codes.OK},
	} {
		_, err := r.vc.CreateVoucher(sellerCtx(t), shopVoucher(tc.code, tc.n, false))
		if code(err) != tc.want {
			t.Errorf("per_user_limit %d: got %v, want %v (err=%v)", tc.n, code(err), tc.want, err)
		}
		_, gerr := r.vouchers.GetByCode(t.Context(), tc.code)
		if (tc.want == codes.OK) != (gerr == nil) {
			t.Errorf("per_user_limit %d: stored row present=%v, want %v", tc.n, gerr == nil, tc.want == codes.OK)
		}
	}
}

func TestPerUserFieldsKeepAuthz(t *testing.T) {
	r := startRig(t, rigOpts{})
	if _, err := r.vc.CreateVoucher(adminCtx(t), platformVoucher("AUTHZ1", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	seeded, err := r.vouchers.GetByCode(t.Context(), "AUTHZ1")
	if err != nil {
		t.Fatal(err)
	}

	for _, scope := range []promotionv1.VoucherScope{promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP, promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM} {
		req := platformVoucher("BUYER-"+scope.String(), scope)
		req.PerUserLimit = 1
		if _, err := r.vc.CreateVoucher(buyerCtx(t), req); code(err) != codes.PermissionDenied {
			t.Errorf("buyer create (%v): got %v, want PermissionDenied", scope, code(err))
		}
		if _, err := r.vouchers.GetByCode(t.Context(), req.GetCode()); err == nil {
			t.Errorf("buyer create (%v) left a voucher row", scope)
		}
	}

	three := int32(3)
	flag := true
	for _, req := range []*promotionv1.UpdateVoucherRequest{
		{VoucherId: seeded.ID, PerUserLimit: &three},
		{VoucherId: seeded.ID, NewUserOnly: &flag},
	} {
		if _, err := r.vc.UpdateVoucher(sellerCtx(t), req); code(err) != codes.PermissionDenied {
			t.Errorf("seller update: got %v, want PermissionDenied", code(err))
		}
		if _, err := r.vc.UpdateVoucher(buyerCtx(t), req); code(err) != codes.PermissionDenied {
			t.Errorf("buyer update: got %v, want PermissionDenied", code(err))
		}
	}
	after, err := r.vouchers.GetByID(t.Context(), seeded.ID)
	if err != nil || after.PerUserLimit != seeded.PerUserLimit || after.NewUserOnly != seeded.NewUserOnly {
		t.Fatalf("stored row changed by refused updates: %+v err=%v", after, err)
	}
}

func TestUpdatePerUserFields(t *testing.T) {
	rec := &recordingAudit{}
	r := startRig(t, rigOpts{audit: rec})
	if _, err := r.vc.CreateVoucher(adminCtx(t), platformVoucher("UPD1", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	std, err := r.vouchers.GetByCode(t.Context(), "UPD1")
	if err != nil {
		t.Fatal(err)
	}
	three := int32(3)
	flag := true
	resp, err := r.vc.UpdateVoucher(adminCtx(t), &promotionv1.UpdateVoucherRequest{VoucherId: std.ID, PerUserLimit: &three, NewUserOnly: &flag})
	if err != nil {
		t.Fatalf("UpdateVoucher: %v", err)
	}
	v := resp.GetVoucher()
	if v.GetPerUserLimit() != 3 || !v.GetNewUserOnly() || v.GetKind() != promotionv1.VoucherKind_VOUCHER_KIND_STANDARD || v.GetOwnerUserId() != "" {
		t.Fatalf("updated = per_user_limit %d new_user_only %v kind %v owner %q", v.GetPerUserLimit(), v.GetNewUserOnly(), v.GetKind(), v.GetOwnerUserId())
	}
	stored, _ := r.vouchers.GetByID(t.Context(), std.ID)
	if stored.PerUserLimit != 3 || !stored.NewUserOnly || stored.Kind != store.VoucherKindStandard || stored.Quota != std.Quota {
		t.Fatalf("stored = %+v", stored)
	}
	var upd map[string]string
	for _, ev := range rec.got() {
		if ev.Action == "voucher.update" {
			upd = ev.Metadata
		}
	}
	if upd["per_user_limit"] != "3" || upd["new_user_only"] != "true" {
		t.Fatalf("update audit metadata = %v, want per_user_limit=3 new_user_only=true", upd)
	}

	for _, bad := range []int32{-1, 1001} {
		n := bad
		if _, err := r.vc.UpdateVoucher(adminCtx(t), &promotionv1.UpdateVoucherRequest{VoucherId: std.ID, PerUserLimit: &n}); code(err) != codes.InvalidArgument {
			t.Errorf("per_user_limit %d: got %v, want InvalidArgument", bad, code(err))
		}
	}
	if again, _ := r.vouchers.GetByID(t.Context(), std.ID); again.PerUserLimit != 3 {
		t.Fatalf("refused update changed per_user_limit to %d", again.PerUserLimit)
	}

	nu := seedOwned(t, r, "NEWU-A", "user-a", nil)
	four := int32(4)
	if _, err := r.vc.UpdateVoucher(adminCtx(t), &promotionv1.UpdateVoucherRequest{VoucherId: nu.ID, PerUserLimit: &four}); code(err) != codes.InvalidArgument {
		t.Fatalf("update of a NEW_USER voucher: got %v, want InvalidArgument", code(err))
	}
	if flagOnly, err := r.vc.UpdateVoucher(adminCtx(t), &promotionv1.UpdateVoucherRequest{VoucherId: nu.ID, NewUserOnly: &flag}); code(err) != codes.InvalidArgument {
		t.Fatalf("new_user_only update of a NEW_USER voucher: got %v (%v), want InvalidArgument", code(err), flagOnly)
	}
	after, _ := r.vouchers.GetByID(t.Context(), nu.ID)
	if after.PerUserLimit != 1 || after.NewUserOnly || after.OwnerUserID != "user-a" || after.Kind != store.VoucherKindNewUser {
		t.Fatalf("NEW_USER voucher changed: %+v", after)
	}
}
