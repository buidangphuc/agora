package voucher

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
)

func (e *adminEnv) seedShop(t *testing.T, code, seller string, scope int32, ends time.Time) store.Voucher {
	t.Helper()
	v, err := e.mem.Create(t.Context(), store.Voucher{
		Code: code, Scope: scope, SellerID: seller, DiscountType: discountTypePercent, DiscountValue: 10,
		MinSpend: 5000, MaxDiscount: 3000, Quota: 5,
		StartsAt: adminNow.Add(-2 * time.Hour), EndsAt: ends,
	}, e.svc.event)
	if err != nil {
		t.Fatalf("seed %s: %v", code, err)
	}
	return v
}

func codesOf(vs []store.Voucher) map[string]bool {
	out := map[string]bool{}
	for _, v := range vs {
		out[v.Code] = true
	}
	return out
}

func TestListMyShopVouchersEveryState_SellerSeesOwnVouchersOfEveryState(t *testing.T) {
	e := newAdminEnv(t)
	live := e.seedShop(t, "A-LIVE", "seller-A", voucherScopeShop, adminNow.Add(24*time.Hour))
	off := e.seedShop(t, "A-OFF", "seller-A", voucherScopeShop, adminNow.Add(24*time.Hour))
	e.seedShop(t, "A-OLD", "seller-A", voucherScopeShop, adminNow.Add(-time.Hour))
	e.seedShop(t, "B-LIVE", "seller-B", voucherScopeShop, adminNow.Add(24*time.Hour))
	e.seedShop(t, "PLAT", "", 2, adminNow.Add(24*time.Hour))
	e.seedShop(t, "PLAT-UNDER-A", "seller-A", 2, adminNow.Add(24*time.Hour))
	if _, err := e.mem.Create(t.Context(), store.Voucher{
		Code: "A-BOUND", Scope: voucherScopeShop, SellerID: "seller-A", OwnerUserID: "u1", Quota: 1,
		StartsAt: adminNow.Add(-time.Hour), EndsAt: adminNow.Add(time.Hour),
	}, e.svc.event); err != nil {
		t.Fatal(err)
	}
	if _, err := e.mem.Deactivate(t.Context(), off.ID, adminNow); err != nil {
		t.Fatal(err)
	}

	got, next, err := e.svc.ListMyShopVouchers(t.Context(), "seller-A", "", 0)
	if err != nil {
		t.Fatalf("ListMyShopVouchers: %v", err)
	}
	codes := codesOf(got)
	if len(got) != 3 || !codes["A-LIVE"] || !codes["A-OFF"] || !codes["A-OLD"] || next != "" {
		t.Fatalf("got %v next %q, want exactly A-LIVE A-OFF A-OLD and no cursor", codes, next)
	}
	for _, v := range got {
		if v.Code == "A-OFF" && (v.Active || v.DeactivatedAt.IsZero()) {
			t.Errorf("A-OFF active=%v deactivated_at=%v, want inactive with timestamp", v.Active, v.DeactivatedAt)
		}
		if v.Code == "A-LIVE" && (!v.Active || v.Quota != live.Quota) {
			t.Errorf("A-LIVE = %+v", v)
		}
	}
	e.w.mu.Lock()
	defer e.w.mu.Unlock()
	if len(e.w.events) != 0 {
		t.Fatalf("list wrote audit events: %+v", e.w.events)
	}
}

func TestListMyShopVouchersEmpty(t *testing.T) {
	e := newAdminEnv(t)
	e.seedShop(t, "B-LIVE", "seller-B", voucherScopeShop, adminNow.Add(time.Hour))
	got, next, err := e.svc.ListMyShopVouchers(t.Context(), "seller-A", "", 0)
	if err != nil {
		t.Fatalf("ListMyShopVouchers: %v", err)
	}
	if len(got) != 0 || next != "" {
		t.Fatalf("got %d vouchers cursor %q, want none and empty cursor", len(got), next)
	}
}

func TestListMyShopVouchersEmptySellerID_SellerSeesOwnVouchersOfEveryState(t *testing.T) {
	e := newAdminEnv(t)
	e.seedShop(t, "PLAT", "", 2, adminNow.Add(time.Hour))
	e.seedShop(t, "SHOP-NOSELLER", "", voucherScopeShop, adminNow.Add(time.Hour))
	got, _, err := e.svc.ListMyShopVouchers(t.Context(), "", "", 0)
	if err == nil && len(got) != 0 {
		t.Fatalf("empty seller id returned %v, want nothing or an error", codesOf(got))
	}
}

func TestListMyShopVouchersPageCap(t *testing.T) {
	e := newAdminEnv(t)
	for i := 0; i < 205; i++ {
		e.seedShop(t, fmt.Sprintf("A-%03d", i), "seller-A", voucherScopeShop, adminNow.Add(time.Hour))
	}
	got, next, err := e.svc.ListMyShopVouchers(t.Context(), "seller-A", "", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > 200 {
		t.Fatalf("page_size 1000 returned %d, want at most 200", len(got))
	}
	if len(got) == 0 || next == "" {
		t.Fatalf("got %d next %q, want a non-empty page with a next cursor (205 exist)", len(got), next)
	}
	rest, next2, err := e.svc.ListMyShopVouchers(t.Context(), "seller-A", next, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(got)+len(rest) != 205 || next2 != "" {
		t.Fatalf("pages hold %d+%d next %q, want 205 total and a last page", len(got), len(rest), next2)
	}
}

func TestDeactivateMyShopVoucher_SellerDeactivatesTheirVoucher(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seedShop(t, "A-LIVE", "seller-A", voucherScopeShop, adminNow.Add(time.Hour))
	got, err := e.svc.DeactivateMyShopVoucher(t.Context(), v.ID, "seller-A")
	if err != nil {
		t.Fatalf("DeactivateMyShopVoucher: %v", err)
	}
	if got.Active || !got.DeactivatedAt.Equal(adminNow) || got.ID != v.ID {
		t.Fatalf("got active=%v deactivated_at=%v, want inactive at %v", got.Active, got.DeactivatedAt, adminNow)
	}
	stored, _ := e.mem.GetByID(t.Context(), v.ID)
	if stored.Active {
		t.Fatal("voucher still active in the store")
	}
	res, err := e.svc.ValidateAndReserve(t.Context(), "r-1", "A-LIVE", "buyer-1", 10000, "seller-A")
	if err != nil || res.Valid {
		t.Fatalf("ValidateAndReserve after deactivate: valid=%v err=%v, want invalid", res.Valid, err)
	}
}

func TestDeactivateMyShopVoucherIdempotent(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seedShop(t, "A-LIVE", "seller-A", voucherScopeShop, adminNow.Add(time.Hour))
	first, err := e.svc.DeactivateMyShopVoucher(t.Context(), v.ID, "seller-A")
	if err != nil {
		t.Fatal(err)
	}
	e.svc.nowFn = func() time.Time { return adminNow.Add(time.Hour) }
	second, err := e.svc.DeactivateMyShopVoucher(t.Context(), v.ID, "seller-A")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if first.DeactivatedAt.IsZero() || !second.DeactivatedAt.Equal(first.DeactivatedAt) {
		t.Fatalf("deactivated_at first=%v second=%v, want identical and set", first.DeactivatedAt, second.DeactivatedAt)
	}
}

func TestDeactivateMyShopVoucherNotOwner_DeactivatingAnotherSellersVoucherIsDeniedLikeAnUnknownId(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seedShop(t, "A-LIVE", "seller-A", voucherScopeShop, adminNow.Add(time.Hour))
	plat := e.seedShop(t, "PLAT", "", 2, adminNow.Add(time.Hour))
	platUnder := e.seedShop(t, "PLAT-A", "seller-A", 2, adminNow.Add(time.Hour))
	bound, err := e.mem.Create(t.Context(), store.Voucher{
		Code: "A-BOUND", Scope: voucherScopeShop, SellerID: "seller-A", OwnerUserID: "u1", Quota: 1,
		StartsAt: adminNow.Add(-time.Hour), EndsAt: adminNow.Add(time.Hour),
	}, e.svc.event)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, id, caller string }{
		{"foreign", v.ID, "seller-B"},
		{"unknown", "no-such-id", "seller-B"},
		{"empty id", "", "seller-A"},
		{"platform scope", plat.ID, "seller-A"},
		{"platform scope under the seller id", platUnder.ID, "seller-A"},
		{"user-bound", bound.ID, "seller-A"},
		{"empty caller", v.ID, ""},
	}
	var msgs []string
	for _, tc := range cases {
		_, err := e.svc.DeactivateMyShopVoucher(t.Context(), tc.id, tc.caller)
		if !errors.Is(err, store.ErrNotOwner) {
			t.Errorf("%s: err = %v, want store.ErrNotOwner", tc.name, err)
			continue
		}
		msgs = append(msgs, err.Error())
	}
	for _, m := range msgs {
		if m != msgs[0] {
			t.Fatalf("denial messages differ: %q vs %q", m, msgs[0])
		}
	}
	for _, id := range []string{v.ID, plat.ID, platUnder.ID, bound.ID} {
		got, _ := e.mem.GetByID(t.Context(), id)
		if !got.Active || !got.DeactivatedAt.IsZero() {
			t.Errorf("%s changed by a denied deactivate: %+v", got.Code, got)
		}
	}
}

func TestDeactivateMyShopVoucherNoAudit_SellerDeactivationWritesNoAuditEvent(t *testing.T) {
	e := newAdminEnv(t)
	e.w.err = errors.New("audit down")
	v := e.seedShop(t, "A-LIVE", "seller-A", voucherScopeShop, adminNow.Add(time.Hour))
	if _, err := e.svc.DeactivateMyShopVoucher(t.Context(), v.ID, "seller-A"); err != nil {
		t.Fatalf("deactivate with audit down: %v", err)
	}
	e.w.mu.Lock()
	defer e.w.mu.Unlock()
	if len(e.w.events) != 0 {
		t.Fatalf("audit events = %+v, want none", e.w.events)
	}
}

func TestDeactivateMyShopVoucherAdminRegression_AdminDeactivateKeepsItsAdminRules(t *testing.T) {
	e := newAdminEnv(t)
	v := e.seedShop(t, "A-LIVE", "seller-A", voucherScopeShop, adminNow.Add(time.Hour))
	if _, err := e.svc.DeactivateVoucher(t.Context(), DeactivateVoucherParams{VoucherID: v.ID, AuditActorID: "admin-1"}); err != nil {
		t.Fatalf("admin deactivate: %v", err)
	}
	e.w.mu.Lock()
	defer e.w.mu.Unlock()
	if len(e.w.events) != 1 || e.w.events[0].Action != "voucher.deactivate" || e.w.events[0].TargetID != "A-LIVE" {
		t.Fatalf("events = %+v, want one voucher.deactivate for A-LIVE", e.w.events)
	}
}
