package storefake_test

import (
	"errors"
	"testing"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
)

func fakeSeed(t *testing.T, repo *storefake.InMemoryVoucherRepository, code string, quota, used int64) store.Voucher {
	t.Helper()
	now := time.Now().UTC()
	v, err := repo.Create(t.Context(), store.Voucher{
		Code: code, Scope: 2, DiscountType: 1, DiscountValue: 10, MinSpend: 100, MaxDiscount: 50,
		Quota: quota, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	}, voucherEvent)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if used > 0 {
		if err := repo.IncrementUsed(t.Context(), v.ID, used); err != nil {
			t.Fatalf("increment: %v", err)
		}
	}
	got, _ := repo.GetByID(t.Context(), v.ID)
	return got
}

func TestInMemoryVoucherCreateIsActive_V1(t *testing.T) {
	repo := storefake.NewInMemoryVoucherRepository()
	v := fakeSeed(t, repo, "ACT1", 5, 0)
	if !v.Active || !v.DeactivatedAt.IsZero() {
		t.Fatalf("active=%v deactivated_at=%v, want active and zero", v.Active, v.DeactivatedAt)
	}
}

func TestInMemoryVoucherDeactivate_V1_V4_V5(t *testing.T) {
	repo := storefake.NewInMemoryVoucherRepository()
	v := fakeSeed(t, repo, "STOP1", 5, 2)
	first := time.Now().UTC()

	got, err := repo.Deactivate(t.Context(), v.ID, first)
	if err != nil || got.Active || !got.DeactivatedAt.Equal(first) {
		t.Fatalf("first Deactivate = %+v err=%v, want inactive at %v", got, err, first)
	}
	got, err = repo.Deactivate(t.Context(), v.ID, first.Add(time.Hour))
	if err != nil || got.Active || !got.DeactivatedAt.Equal(first) {
		t.Fatalf("second Deactivate = %+v err=%v, want original %v", got, err, first)
	}
	stored, _ := repo.GetByID(t.Context(), v.ID)
	if stored.Active || !stored.DeactivatedAt.Equal(first) || stored.Quota != 5 || stored.Used != 2 {
		t.Fatalf("stored = %+v", stored)
	}
	if _, err := repo.Deactivate(t.Context(), "missing", first); !errors.Is(err, store.ErrVoucherNotFound) {
		t.Fatalf("unknown: err=%v, want ErrVoucherNotFound", err)
	}
}

func TestInMemoryVoucherUpdateEndsAtQuota_V8_V9_V12b(t *testing.T) {
	repo := storefake.NewInMemoryVoucherRepository()
	before := fakeSeed(t, repo, "UPD1", 100, 40)
	newEnd := before.EndsAt.Add(24 * time.Hour)

	got, err := repo.UpdateEndsAtQuota(t.Context(), before.ID, time.Time{}, 150)
	if err != nil || got.Quota != 150 || !got.EndsAt.Equal(before.EndsAt) {
		t.Fatalf("raise quota = %+v err=%v", got, err)
	}
	got, err = repo.UpdateEndsAtQuota(t.Context(), before.ID, newEnd, 0)
	if err != nil || got.Quota != 150 || !got.EndsAt.Equal(newEnd) {
		t.Fatalf("move end = %+v err=%v", got, err)
	}
	got, err = repo.UpdateEndsAtQuota(t.Context(), before.ID, newEnd.Add(time.Hour), 150)
	if err != nil || got.Quota != 150 || !got.EndsAt.Equal(newEnd.Add(time.Hour)) {
		t.Fatalf("same quota with end = %+v err=%v", got, err)
	}
	stored, _ := repo.GetByID(t.Context(), before.ID)
	if stored.DiscountType != before.DiscountType || stored.DiscountValue != before.DiscountValue ||
		stored.MinSpend != before.MinSpend || stored.MaxDiscount != before.MaxDiscount ||
		stored.Scope != before.Scope || stored.Used != before.Used || !stored.Active {
		t.Fatalf("immutable fields changed: before=%+v after=%+v", before, stored)
	}
}

func TestInMemoryVoucherUpdateRefusals_V10_V11_V15_V16b(t *testing.T) {
	repo := storefake.NewInMemoryVoucherRepository()
	v := fakeSeed(t, repo, "REF1", 100, 40)
	for _, quota := range []int64{30, 90} {
		if _, err := repo.UpdateEndsAtQuota(t.Context(), v.ID, time.Time{}, quota); !errors.Is(err, store.ErrVoucherQuota) {
			t.Fatalf("quota %d: err=%v, want ErrVoucherQuota", quota, err)
		}
	}
	if stored, _ := repo.GetByID(t.Context(), v.ID); stored.Quota != 100 {
		t.Fatalf("quota=%d after refusals, want 100", stored.Quota)
	}

	unl := fakeSeed(t, repo, "UNL1", 0, 3)
	if _, err := repo.UpdateEndsAtQuota(t.Context(), unl.ID, time.Time{}, 500); !errors.Is(err, store.ErrVoucherQuota) {
		t.Fatalf("finite on unlimited: err=%v, want ErrVoucherQuota", err)
	}
	newEnd := unl.EndsAt.Add(time.Hour)
	if got, err := repo.UpdateEndsAtQuota(t.Context(), unl.ID, newEnd, 0); err != nil || got.Quota != 0 || !got.EndsAt.Equal(newEnd) {
		t.Fatalf("unlimited end move = %+v err=%v", got, err)
	}

	if _, err := repo.Deactivate(t.Context(), v.ID, time.Now().UTC()); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	if _, err := repo.UpdateEndsAtQuota(t.Context(), v.ID, time.Time{}, 200); !errors.Is(err, store.ErrVoucherInactive) {
		t.Fatalf("inactive: err=%v, want ErrVoucherInactive", err)
	}
	if _, err := repo.UpdateEndsAtQuota(t.Context(), "missing", time.Time{}, 200); !errors.Is(err, store.ErrVoucherNotFound) {
		t.Fatalf("unknown: err=%v, want ErrVoucherNotFound", err)
	}
}
