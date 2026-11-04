package store_test

import (
	"errors"
	"io/fs"
	"sync"
	"testing"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
)

func adminSeed(t *testing.T, repo *store.PostgresVoucherRepository, code string, quota, used int64) store.Voucher {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	v, err := repo.Create(t.Context(), store.Voucher{
		Code: code, Scope: 2, DiscountType: 1, DiscountValue: 10, MinSpend: 100, MaxDiscount: 50,
		Quota: quota, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	}, voucherEvent)
	if err != nil {
		t.Fatalf("create %s: %v", code, err)
	}
	if used > 0 {
		if err := repo.IncrementUsed(t.Context(), v.ID, used); err != nil {
			t.Fatalf("increment %s: %v", code, err)
		}
	}
	got, err := repo.GetByID(t.Context(), v.ID)
	if err != nil {
		t.Fatalf("reload %s: %v", code, err)
	}
	return got
}

func assertMoneyFieldsUnchanged(t *testing.T, before, after store.Voucher) {
	t.Helper()
	if before.DiscountType != after.DiscountType || before.DiscountValue != after.DiscountValue ||
		before.MinSpend != after.MinSpend || before.MaxDiscount != after.MaxDiscount ||
		before.Scope != after.Scope || before.SellerID != after.SellerID || before.Code != after.Code ||
		!before.StartsAt.Equal(after.StartsAt) || before.Used != after.Used {
		t.Fatalf("immutable fields changed:\nbefore=%+v\nafter=%+v", before, after)
	}
}

func TestPostgresVoucherCreateIsActive_V1(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	v := adminSeed(t, repo, "ACT1", 5, 0)
	if !v.Active || !v.DeactivatedAt.IsZero() {
		t.Fatalf("new voucher active=%v deactivated_at=%v, want active and zero", v.Active, v.DeactivatedAt)
	}
}

func TestPostgresVoucherDeactivate_V1(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	before := adminSeed(t, repo, "STOP1", 5, 2)
	at := time.Now().UTC().Truncate(time.Microsecond)

	got, err := repo.Deactivate(t.Context(), before.ID, at)
	if err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	if got.Active || !got.DeactivatedAt.Equal(at) {
		t.Fatalf("returned active=%v deactivated_at=%v, want false and %v", got.Active, got.DeactivatedAt, at)
	}
	stored, err := repo.GetByCode(t.Context(), "STOP1")
	if err != nil {
		t.Fatalf("GetByCode: %v", err)
	}
	if stored.Active || !stored.DeactivatedAt.Equal(at) {
		t.Fatalf("stored active=%v deactivated_at=%v, want false and %v", stored.Active, stored.DeactivatedAt, at)
	}
	assertMoneyFieldsUnchanged(t, before, stored)
	if stored.Quota != before.Quota || !stored.EndsAt.Equal(before.EndsAt) {
		t.Fatalf("deactivate changed quota/ends_at: %+v", stored)
	}
}

func TestPostgresVoucherDeactivateTwiceKeepsOriginalTimestamp_V4(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	v := adminSeed(t, repo, "STOP2", 5, 0)
	first := time.Now().UTC().Truncate(time.Microsecond)

	if _, err := repo.Deactivate(t.Context(), v.ID, first); err != nil {
		t.Fatalf("first Deactivate: %v", err)
	}
	got, err := repo.Deactivate(t.Context(), v.ID, first.Add(time.Hour))
	if err != nil {
		t.Fatalf("second Deactivate: %v", err)
	}
	if got.Active || !got.DeactivatedAt.Equal(first) {
		t.Fatalf("second call returned active=%v deactivated_at=%v, want false and original %v", got.Active, got.DeactivatedAt, first)
	}
	stored, _ := repo.GetByID(t.Context(), v.ID)
	if !stored.DeactivatedAt.Equal(first) {
		t.Fatalf("stored deactivated_at=%v, want original %v", stored.DeactivatedAt, first)
	}
}

func TestPostgresVoucherDeactivateUnknown_V5(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	if _, err := repo.Deactivate(t.Context(), "missing", time.Now().UTC()); !errors.Is(err, store.ErrVoucherNotFound) {
		t.Fatalf("err=%v, want ErrVoucherNotFound", err)
	}
}

func TestPostgresVoucherUpdateRaiseQuota_V8(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	before := adminSeed(t, repo, "Q150", 100, 40)

	got, err := repo.UpdateEndsAtQuota(t.Context(), before.ID, time.Time{}, 150)
	if err != nil {
		t.Fatalf("UpdateEndsAtQuota: %v", err)
	}
	if got.Quota != 150 {
		t.Fatalf("returned quota=%d, want 150", got.Quota)
	}
	stored, _ := repo.GetByID(t.Context(), before.ID)
	if stored.Quota != 150 || !stored.EndsAt.Equal(before.EndsAt) || !stored.Active {
		t.Fatalf("stored %+v, want quota 150, ends_at and active unchanged", stored)
	}
	assertMoneyFieldsUnchanged(t, before, stored)
}

func TestPostgresVoucherUpdateMoveEndsAt_V9(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	before := adminSeed(t, repo, "END1", 100, 40)
	newEnd := before.EndsAt.Add(48 * time.Hour).Truncate(time.Microsecond)

	got, err := repo.UpdateEndsAtQuota(t.Context(), before.ID, newEnd, 0)
	if err != nil {
		t.Fatalf("UpdateEndsAtQuota: %v", err)
	}
	if !got.EndsAt.Equal(newEnd) || got.Quota != 100 {
		t.Fatalf("returned ends_at=%v quota=%d, want %v and 100", got.EndsAt, got.Quota, newEnd)
	}
	stored, _ := repo.GetByID(t.Context(), before.ID)
	if !stored.EndsAt.Equal(newEnd) || stored.Quota != 100 {
		t.Fatalf("stored ends_at=%v quota=%d, want %v and 100", stored.EndsAt, stored.Quota, newEnd)
	}
	assertMoneyFieldsUnchanged(t, before, stored)
}

func TestPostgresVoucherUpdateBothFields_V8_V9(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	before := adminSeed(t, repo, "BOTH1", 100, 40)
	newEnd := before.EndsAt.Add(time.Hour).Truncate(time.Microsecond)

	if _, err := repo.UpdateEndsAtQuota(t.Context(), before.ID, newEnd, 120); err != nil {
		t.Fatalf("UpdateEndsAtQuota: %v", err)
	}
	stored, _ := repo.GetByID(t.Context(), before.ID)
	if stored.Quota != 120 || !stored.EndsAt.Equal(newEnd) {
		t.Fatalf("stored quota=%d ends_at=%v, want 120 and %v", stored.Quota, stored.EndsAt, newEnd)
	}
	assertMoneyFieldsUnchanged(t, before, stored)
}

func TestPostgresVoucherUpdateQuotaEqualToCurrentWithEndsAt_V12b(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	before := adminSeed(t, repo, "FULL1", 10, 10)
	newEnd := before.EndsAt.Add(time.Hour).Truncate(time.Microsecond)

	got, err := repo.UpdateEndsAtQuota(t.Context(), before.ID, newEnd, 10)
	if err != nil {
		t.Fatalf("UpdateEndsAtQuota: %v", err)
	}
	if got.Quota != 10 || !got.EndsAt.Equal(newEnd) {
		t.Fatalf("returned quota=%d ends_at=%v, want 10 and %v", got.Quota, got.EndsAt, newEnd)
	}
}

func TestPostgresVoucherUpdateRefusedQuotaLeavesRowUnchanged_V10_V11(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	before := adminSeed(t, repo, "REF1", 100, 40)
	newEnd := before.EndsAt.Add(time.Hour).Truncate(time.Microsecond)

	for name, quota := range map[string]int64{"below used": 30, "below current": 90, "below used and current": 1} {
		_, err := repo.UpdateEndsAtQuota(t.Context(), before.ID, newEnd, quota)
		if !errors.Is(err, store.ErrVoucherQuota) {
			t.Fatalf("%s: err=%v, want ErrVoucherQuota", name, err)
		}
		stored, _ := repo.GetByID(t.Context(), before.ID)
		if stored.Quota != 100 || !stored.EndsAt.Equal(before.EndsAt) {
			t.Fatalf("%s: refused update changed row: %+v", name, stored)
		}
	}
}

func TestPostgresVoucherUpdateUnlimitedCannotBecomeFinite_V16b_V16c(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	before := adminSeed(t, repo, "UNL1", 0, 7)

	if _, err := repo.UpdateEndsAtQuota(t.Context(), before.ID, time.Time{}, 500); !errors.Is(err, store.ErrVoucherQuota) {
		t.Fatalf("finite quota on unlimited: err=%v, want ErrVoucherQuota", err)
	}
	stored, _ := repo.GetByID(t.Context(), before.ID)
	if stored.Quota != 0 {
		t.Fatalf("quota=%d after refused update, want 0", stored.Quota)
	}

	newEnd := before.EndsAt.Add(time.Hour).Truncate(time.Microsecond)
	got, err := repo.UpdateEndsAtQuota(t.Context(), before.ID, newEnd, 0)
	if err != nil {
		t.Fatalf("move end date on unlimited: %v", err)
	}
	if got.Quota != 0 || !got.EndsAt.Equal(newEnd) {
		t.Fatalf("returned quota=%d ends_at=%v, want 0 and %v", got.Quota, got.EndsAt, newEnd)
	}
}

func TestPostgresVoucherUpdateInactiveAndUnknown_V15(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	v := adminSeed(t, repo, "INACT1", 10, 0)
	if _, err := repo.Deactivate(t.Context(), v.ID, time.Now().UTC()); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	if _, err := repo.UpdateEndsAtQuota(t.Context(), v.ID, time.Time{}, 20); !errors.Is(err, store.ErrVoucherInactive) {
		t.Fatalf("update inactive: err=%v, want ErrVoucherInactive", err)
	}
	stored, _ := repo.GetByID(t.Context(), v.ID)
	if stored.Quota != 10 {
		t.Fatalf("quota=%d after refused update, want 10", stored.Quota)
	}
	if _, err := repo.UpdateEndsAtQuota(t.Context(), "missing", time.Time{}, 20); !errors.Is(err, store.ErrVoucherNotFound) {
		t.Fatalf("update unknown: err=%v, want ErrVoucherNotFound", err)
	}
}

func TestPostgresVoucherUpdateChecksStoredUsedNotCallerSnapshot_V16d(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	stale := adminSeed(t, repo, "RACE1", 10, 0)
	if err := repo.IncrementUsed(t.Context(), stale.ID, 10); err != nil {
		t.Fatalf("IncrementUsed: %v", err)
	}

	_, err := repo.UpdateEndsAtQuota(t.Context(), stale.ID, time.Time{}, 9)
	if !errors.Is(err, store.ErrVoucherQuota) {
		t.Fatalf("err=%v, want ErrVoucherQuota", err)
	}
	stored, _ := repo.GetByID(t.Context(), stale.ID)
	if stored.Quota != 10 || stored.Used != 10 {
		t.Fatalf("stored quota=%d used=%d, want 10 and 10", stored.Quota, stored.Used)
	}
}

func TestPostgresVoucherConcurrentUpdatesNeverLowerQuotaBelowUsed_V16d(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	v := adminSeed(t, repo, "RACE2", 10, 0)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := repo.IncrementUsed(t.Context(), v.ID, 1); err != nil {
				t.Errorf("IncrementUsed: %v", err)
			}
		}()
		go func(q int64) {
			defer wg.Done()
			if _, err := repo.UpdateEndsAtQuota(t.Context(), v.ID, time.Time{}, q); err != nil && !errors.Is(err, store.ErrVoucherQuota) {
				t.Errorf("UpdateEndsAtQuota(%d): %v", q, err)
			}
		}(int64(10 + i))
	}
	wg.Wait()

	stored, _ := repo.GetByID(t.Context(), v.ID)
	if stored.Used != 10 {
		t.Fatalf("used=%d, want 10", stored.Used)
	}
	if stored.Quota < stored.Used {
		t.Fatalf("quota=%d below used=%d", stored.Quota, stored.Used)
	}
	if stored.Quota < 10 || stored.Quota > 19 {
		t.Fatalf("quota=%d, want within 10..19", stored.Quota)
	}
}

func TestPostgresVoucherConcurrentDeactivateAndUpdateStayConsistent_V16f(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	for i := 0; i < 20; i++ {
		code := "CONC" + string(rune('A'+i))
		v := adminSeed(t, repo, code, 10, 0)
		var updErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := repo.Deactivate(t.Context(), v.ID, time.Now().UTC()); err != nil {
				t.Errorf("Deactivate: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			_, updErr = repo.UpdateEndsAtQuota(t.Context(), v.ID, time.Time{}, 20)
		}()
		wg.Wait()

		stored, _ := repo.GetByID(t.Context(), v.ID)
		if stored.Active {
			t.Fatalf("%s still active after Deactivate", code)
		}
		switch {
		case updErr == nil && stored.Quota != 20:
			t.Fatalf("%s: update reported success but quota=%d", code, stored.Quota)
		case errors.Is(updErr, store.ErrVoucherInactive) && stored.Quota != 10:
			t.Fatalf("%s: update refused as inactive but quota=%d", code, stored.Quota)
		case updErr != nil && !errors.Is(updErr, store.ErrVoucherInactive):
			t.Fatalf("%s: unexpected update error %v", code, updErr)
		}
	}
}

func voucherActiveMigration(t *testing.T, suffix string) string {
	t.Helper()
	matches, err := fs.Glob(store.Migrations, "migrations/0006_*."+suffix+".sql")
	if err != nil || len(matches) != 1 {
		t.Fatalf("0006 %s migration matches=%v err=%v, want exactly one", suffix, matches, err)
	}
	body, err := fs.ReadFile(store.Migrations, matches[0])
	if err != nil {
		t.Fatalf("read %s: %v", matches[0], err)
	}
	return string(body)
}

func TestVoucherActiveMigrationDefaultsExistingRowsToActive_V1(t *testing.T) {
	pool := newPoolUpTo(t, 5)
	ctx := t.Context()
	if _, err := pool.Exec(ctx, `INSERT INTO vouchers (id, code, scope, quota, used) VALUES ('v-pre', 'PRE10', 2, 5, 1)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := pool.Exec(ctx, voucherActiveMigration(t, "up")); err != nil {
		t.Fatalf("apply 0006 up: %v", err)
	}
	var active bool
	var deactivatedAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT active, deactivated_at FROM vouchers WHERE id = 'v-pre'`).Scan(&active, &deactivatedAt); err != nil {
		t.Fatalf("read migrated row: %v", err)
	}
	if !active || deactivatedAt != nil {
		t.Fatalf("migrated row active=%v deactivated_at=%v, want true and NULL", active, deactivatedAt)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO vouchers (id, code) VALUES ('v-new', 'NEW10')`); err != nil {
		t.Fatalf("insert without active: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT active FROM vouchers WHERE id = 'v-new'`).Scan(&active); err != nil || !active {
		t.Fatalf("default active=%v err=%v, want true", active, err)
	}

	if _, err := pool.Exec(ctx, voucherActiveMigration(t, "down")); err != nil {
		t.Fatalf("apply 0006 down: %v", err)
	}
	var cols int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'vouchers' AND column_name IN ('active', 'deactivated_at')`).Scan(&cols); err != nil || cols != 0 {
		t.Fatalf("columns after down = %d err=%v, want 0", cols, err)
	}
	var code string
	if err := pool.QueryRow(ctx, `SELECT code FROM vouchers WHERE id = 'v-pre'`).Scan(&code); err != nil || code != "PRE10" {
		t.Fatalf("voucher after down = %q err=%v, want kept", code, err)
	}
}
