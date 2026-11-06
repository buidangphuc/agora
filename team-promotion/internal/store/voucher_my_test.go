package store_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
)

var myNow = time.Date(2026, 11, 6, 3, 0, 0, 0, time.UTC)

func newUserVoucher(code, owner string, endsAt time.Time) store.Voucher {
	return store.Voucher{
		Code: code, Scope: 2, DiscountType: 2, DiscountValue: 20000, MinSpend: 100000, Quota: 1,
		PerUserLimit: 1, OwnerUserID: owner, Kind: store.VoucherKindNewUser, Active: true,
		StartsAt: myNow.Add(-time.Hour), EndsAt: endsAt,
	}
}

func TestListByOwnerLimit(t *testing.T) {
	pool := newTestPool(t)
	ctx := t.Context()
	vrepo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	for i := 0; i < 60; i++ {
		v := newUserVoucher(fmt.Sprintf("OWN-%02d", i), "owner-1", myNow.Add(time.Duration(60-i)*time.Hour))
		v.Kind = store.VoucherKindStandard
		if _, err := vrepo.Create(ctx, v, voucherEvent); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	got, err := vrepo.ListByOwner(ctx, "owner-1", myNow, 50)
	if err != nil {
		t.Fatalf("ListByOwner: %v", err)
	}
	if len(got) != 50 {
		t.Fatalf("len = %d, want 50", len(got))
	}
	if !sort.SliceIsSorted(got, func(i, j int) bool { return got[i].EndsAt.Before(got[j].EndsAt) }) {
		t.Fatal("result is not ordered by ends_at ascending")
	}
	if got[0].Code != "OWN-59" || got[49].Code != "OWN-10" {
		t.Fatalf("soonest-expiry-first window = %s .. %s, want OWN-59 .. OWN-10", got[0].Code, got[49].Code)
	}
	for _, v := range got {
		if v.OwnerUserID != "owner-1" {
			t.Fatalf("voucher %s owner = %q", v.Code, v.OwnerUserID)
		}
	}
}

func TestListByOwnerOrdersByEndsAtThenCode(t *testing.T) {
	pool := newTestPool(t)
	ctx := t.Context()
	vrepo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	same := myNow.Add(48 * time.Hour)
	for _, c := range []struct {
		code string
		end  time.Time
	}{{"B-CODE", same}, {"A-CODE", same}, {"Z-FIRST", myNow.Add(time.Hour)}} {
		v := newUserVoucher(c.code, "owner-1", c.end)
		v.Kind = store.VoucherKindStandard
		if _, err := vrepo.Create(ctx, v, voucherEvent); err != nil {
			t.Fatalf("create %s: %v", c.code, err)
		}
	}
	got, err := vrepo.ListByOwner(ctx, "owner-1", myNow, 50)
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, v := range got {
		codes = append(codes, v.Code)
	}
	if fmt.Sprint(codes) != "[Z-FIRST A-CODE B-CODE]" {
		t.Fatalf("order = %v, want [Z-FIRST A-CODE B-CODE]", codes)
	}
}

func TestListByOwnerFilters(t *testing.T) {
	pool := newTestPool(t)
	ctx := t.Context()
	vrepo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	mk := func(code string, mod func(*store.Voucher)) {
		v := newUserVoucher(code, "owner-1", myNow.Add(time.Hour))
		v.Kind = store.VoucherKindStandard
		mod(&v)
		created, err := vrepo.Create(ctx, v, voucherEvent)
		if err != nil {
			t.Fatalf("create %s: %v", code, err)
		}
		if code == "USED" {
			if err := vrepo.IncrementUsed(ctx, created.ID, 1); err != nil {
				t.Fatal(err)
			}
		}
		if code == "OFF" {
			if _, err := vrepo.Deactivate(ctx, created.ID, myNow); err != nil {
				t.Fatal(err)
			}
		}
	}
	mk("USED", func(*store.Voucher) {})
	mk("EXPIRED", func(v *store.Voucher) { v.EndsAt = myNow })
	mk("OFF", func(*store.Voucher) {})
	mk("FUTURE", func(v *store.Voucher) { v.StartsAt = myNow.Add(time.Hour) })
	mk("OTHER", func(v *store.Voucher) { v.OwnerUserID = "owner-2" })
	mk("LIVE", func(v *store.Voucher) { v.EndsAt = myNow.Add(time.Second) })
	mk("UNLIMITED", func(v *store.Voucher) { v.Quota = 0 })

	got, err := vrepo.ListByOwner(ctx, "owner-1", myNow, 50)
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, v := range got {
		codes = append(codes, v.Code)
	}
	if fmt.Sprint(codes) != "[LIVE UNLIMITED]" {
		t.Fatalf("visible = %v, want [LIVE UNLIMITED]", codes)
	}
	empty, err := vrepo.ListByOwner(ctx, "nobody", myNow, 50)
	if err != nil || len(empty) != 0 {
		t.Fatalf("owner without vouchers = %v err=%v, want empty", empty, err)
	}
}

func TestListExcludesOwned(t *testing.T) {
	pool := newTestPool(t)
	ctx := t.Context()
	vrepo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	for i := 0; i < 3; i++ {
		if _, err := vrepo.Create(ctx, newUserVoucher(fmt.Sprintf("NEWU-%d", i), fmt.Sprintf("user-%d", i), myNow.Add(time.Hour)), voucherEvent); err != nil {
			t.Fatalf("create new-user %d: %v", i, err)
		}
	}
	for i := 0; i < 2; i++ {
		v := store.Voucher{Code: fmt.Sprintf("PUB-%d", i), Scope: 2, DiscountType: 2, DiscountValue: 1000, Quota: 5, Active: true}
		if _, err := vrepo.Create(ctx, v, voucherEvent); err != nil {
			t.Fatalf("create public %d: %v", i, err)
		}
	}
	got, err := vrepo.List(ctx, "", 50, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("public list has %d vouchers, want 2: %+v", len(got), got)
	}
	for _, v := range got {
		if v.OwnerUserID != "" {
			t.Fatalf("public list leaked owner %q on %s", v.OwnerUserID, v.Code)
		}
	}
	byCode, err := vrepo.GetByCode(ctx, "NEWU-0")
	if err != nil || byCode.OwnerUserID != "user-0" || byCode.Kind != store.VoucherKindNewUser {
		t.Fatalf("GetByCode owner view = %+v err=%v, want owner user-0 kind NEW_USER", byCode, err)
	}
}
