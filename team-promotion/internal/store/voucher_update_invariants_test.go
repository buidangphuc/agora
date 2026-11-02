package store_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
)

func seedScoped(t *testing.T, repo *store.PostgresVoucherRepository, code string, scope int32, seller string, quota int64) store.Voucher {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	v, err := repo.Create(t.Context(), store.Voucher{
		Code: code, Scope: scope, SellerID: seller, DiscountType: 2, DiscountValue: 15, MinSpend: 250, MaxDiscount: 75,
		Quota: quota, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	}, voucherEvent)
	if err != nil {
		t.Fatalf("create %s: %v", code, err)
	}
	return v
}

func TestPostgresVoucherUpdateKeepsMoneyAndScopeForEveryInput_V16(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	later := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Microsecond)

	cases := []struct {
		name    string
		scope   int32
		seller  string
		quota   int64
		endsAt  time.Time
		newQuot int64
	}{
		{"platform quota only", 1, "", 100, time.Time{}, 200},
		{"platform ends only", 1, "", 100, later, 0},
		{"platform both", 1, "", 100, later, 100},
		{"seller quota only", 2, "seller-a", 50, time.Time{}, 51},
		{"seller both", 2, "seller-a", 50, later, 500},
		{"unlimited ends only", 2, "seller-b", 0, later, 0},
		{"no-op keep both", 1, "", 10, time.Time{}, 0},
	}
	for i, c := range cases {
		code := "INV" + string(rune('A'+i))
		before := seedScoped(t, repo, code, c.scope, c.seller, c.quota)

		returned, err := repo.UpdateEndsAtQuota(t.Context(), before.ID, c.endsAt, c.newQuot)
		if err != nil {
			t.Fatalf("%s: UpdateEndsAtQuota: %v", c.name, err)
		}
		stored, err := repo.GetByID(t.Context(), before.ID)
		if err != nil {
			t.Fatalf("%s: reload: %v", c.name, err)
		}
		assertMoneyFieldsUnchanged(t, before, stored)
		assertMoneyFieldsUnchanged(t, before, returned)
		if !stored.Active || !stored.DeactivatedAt.IsZero() {
			t.Fatalf("%s: update changed activation: %+v", c.name, stored)
		}
	}
}

func TestPostgresVoucherRefusedUpdateKeepsMoneyScopeAndEndsAt_V16_V16d(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	before := seedScoped(t, repo, "REFUSED", 2, "seller-z", 20)
	if err := repo.IncrementUsed(t.Context(), before.ID, 15); err != nil {
		t.Fatalf("IncrementUsed: %v", err)
	}
	before, _ = repo.GetByID(t.Context(), before.ID)

	_, err := repo.UpdateEndsAtQuota(t.Context(), before.ID, time.Now().UTC().Add(240*time.Hour), 12)
	if !errors.Is(err, store.ErrVoucherQuota) {
		t.Fatalf("err=%v, want ErrVoucherQuota", err)
	}
	stored, _ := repo.GetByID(t.Context(), before.ID)
	assertMoneyFieldsUnchanged(t, before, stored)
	if stored.Quota != 20 || !stored.EndsAt.Equal(before.EndsAt) {
		t.Fatalf("refused update leaked a partial write: quota=%d ends_at=%v", stored.Quota, stored.EndsAt)
	}
}

func TestPostgresVoucherUpdateAgainstConcurrentReservationsNeverBreaksQuotaInvariant_V16d(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	const total = 40
	v := seedScoped(t, repo, "RACE3", 1, "", total)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var accepted []int64
	for i := 0; i < total; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := repo.IncrementUsed(t.Context(), v.ID, 1); err != nil {
				t.Errorf("IncrementUsed: %v", err)
			}
		}()
		go func(q int64) {
			defer wg.Done()
			got, err := repo.UpdateEndsAtQuota(t.Context(), v.ID, time.Time{}, q)
			switch {
			case err == nil:
				if got.Quota != q || got.Quota < got.Used {
					t.Errorf("accepted update returned quota=%d used=%d for requested %d", got.Quota, got.Used, q)
				}
				mu.Lock()
				accepted = append(accepted, q)
				mu.Unlock()
			case errors.Is(err, store.ErrVoucherQuota):
			default:
				t.Errorf("UpdateEndsAtQuota(%d): %v", q, err)
			}
		}(int64(total + i))
	}
	wg.Wait()

	stored, _ := repo.GetByID(t.Context(), v.ID)
	if stored.Used != total {
		t.Fatalf("used=%d, want %d", stored.Used, total)
	}
	if stored.Quota < stored.Used {
		t.Fatalf("quota=%d below used=%d", stored.Quota, stored.Used)
	}
	if stored.Quota < total || stored.Quota >= 2*total {
		t.Fatalf("quota=%d outside the requested range %d..%d", stored.Quota, total, 2*total-1)
	}
	assertMoneyFieldsUnchanged(t, store.Voucher{
		Code: v.Code, Scope: v.Scope, SellerID: v.SellerID, DiscountType: v.DiscountType, DiscountValue: v.DiscountValue,
		MinSpend: v.MinSpend, MaxDiscount: v.MaxDiscount, StartsAt: v.StartsAt, Used: total,
	}, store.Voucher{
		Code: stored.Code, Scope: stored.Scope, SellerID: stored.SellerID, DiscountType: stored.DiscountType, DiscountValue: stored.DiscountValue,
		MinSpend: stored.MinSpend, MaxDiscount: stored.MaxDiscount, StartsAt: stored.StartsAt, Used: stored.Used,
	})
}

func TestPostgresVoucherUpdateCannotLowerBelowUsedEvenAtExactBoundary_V16d(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	v := seedScoped(t, repo, "BOUND", 1, "", 10)
	if err := repo.IncrementUsed(t.Context(), v.ID, 10); err != nil {
		t.Fatalf("IncrementUsed: %v", err)
	}

	got, err := repo.UpdateEndsAtQuota(t.Context(), v.ID, time.Time{}, 10)
	if err != nil {
		t.Fatalf("quota == used == current must be accepted: %v", err)
	}
	if got.Quota != 10 || got.Used != 10 {
		t.Fatalf("quota=%d used=%d, want 10 and 10", got.Quota, got.Used)
	}
	if _, err := repo.UpdateEndsAtQuota(t.Context(), v.ID, time.Time{}, 9); !errors.Is(err, store.ErrVoucherQuota) {
		t.Fatalf("quota 9 with used 10: err=%v, want ErrVoucherQuota", err)
	}
}

func TestPostgresVoucherDeactivateWinsOverLateUpdateAndKeepsMoneyFields_V16f(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	before := seedScoped(t, repo, "LATE", 2, "seller-l", 10)
	if _, err := repo.Deactivate(t.Context(), before.ID, time.Now().UTC()); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}

	_, err := repo.UpdateEndsAtQuota(t.Context(), before.ID, time.Now().UTC().Add(96*time.Hour), 99)
	if !errors.Is(err, store.ErrVoucherInactive) {
		t.Fatalf("err=%v, want ErrVoucherInactive", err)
	}
	stored, _ := repo.GetByID(t.Context(), before.ID)
	assertMoneyFieldsUnchanged(t, before, stored)
	if stored.Quota != 10 || !stored.EndsAt.Equal(before.EndsAt) || stored.Active {
		t.Fatalf("late update altered a deactivated voucher: %+v", stored)
	}
}
