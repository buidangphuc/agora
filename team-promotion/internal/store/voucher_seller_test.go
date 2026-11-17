package store_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
)

func TestListMyShopVouchersStore_SellerSeesOwnVouchersOfEveryState(t *testing.T) {
	pool := newTestPool(t)
	ctx := t.Context()
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	now := time.Now().UTC().Truncate(time.Microsecond)
	mk := func(code, seller string, scope int32, owner string, ends time.Time) store.Voucher {
		v, err := repo.Create(ctx, store.Voucher{
			Code: code, Scope: scope, SellerID: seller, OwnerUserID: owner, DiscountType: 1, DiscountValue: 10,
			Quota: 5, StartsAt: now.Add(-2 * time.Hour), EndsAt: ends,
		}, voucherEvent)
		if err != nil {
			t.Fatalf("create %s: %v", code, err)
		}
		return v
	}
	mk("A-1", "seller-A", 1, "", now.Add(time.Hour))
	off := mk("A-2", "seller-A", 1, "", now.Add(time.Hour))
	mk("A-3", "seller-A", 1, "", now.Add(-time.Hour))
	mk("B-1", "seller-B", 1, "", now.Add(time.Hour))
	mk("PLAT", "", 2, "", now.Add(time.Hour))
	mk("PLAT-A", "seller-A", 2, "", now.Add(time.Hour))
	mk("A-BOUND", "seller-A", 1, "user-1", now.Add(time.Hour))
	if _, err := repo.Deactivate(ctx, off.ID, now); err != nil {
		t.Fatal(err)
	}

	got, err := repo.List(ctx, "seller-A", 50, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var codes []string
	for _, v := range got {
		codes = append(codes, v.Code)
		if v.Scope != 1 || v.SellerID != "seller-A" || v.OwnerUserID != "" {
			t.Errorf("%s leaked into the seller list: %+v", v.Code, v)
		}
	}
	if fmt.Sprint(codes) != "[A-3 A-2 A-1]" {
		t.Fatalf("codes = %v, want [A-3 A-2 A-1] (every state, newest first, no platform or user-bound)", codes)
	}
}
