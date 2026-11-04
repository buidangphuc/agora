package store_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
)

func dupVoucher(code string) store.Voucher {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return store.Voucher{
		Code: code, Scope: 2, DiscountType: 1, DiscountValue: 10, MinSpend: 100, MaxDiscount: 50,
		Quota: 5, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	}
}

func TestPostgresVoucherCreateDuplicateCodeIsTyped_V18(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	if _, err := repo.Create(t.Context(), dupVoucher("DUPT1"), voucherEvent); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := repo.Create(t.Context(), dupVoucher("DUPT1"), voucherEvent)
	if !errors.Is(err, store.ErrDuplicateCode) {
		t.Fatalf("second create err = %v, want errors.Is ErrDuplicateCode", err)
	}
}

func TestPostgresVoucherCreateOtherFailureIsNotDuplicate_V18(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	bad := dupVoucher("DUPT2")
	bad.DiscountType = -1
	_, err := repo.Create(t.Context(), bad, voucherEvent)
	if errors.Is(err, store.ErrDuplicateCode) {
		t.Fatalf("non-duplicate failure mapped to ErrDuplicateCode: %v", err)
	}
}

func TestPostgresVoucherCreateRaceOneWinnerTypedLoser_V18b_V18c(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = repo.Create(t.Context(), dupVoucher("DUPRACE"), voucherEvent)
		}()
	}
	close(start)
	wg.Wait()
	wins := 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case !errors.Is(err, store.ErrDuplicateCode):
			t.Fatalf("loser err = %v, want ErrDuplicateCode", err)
		}
	}
	if wins != 1 {
		t.Fatalf("winners = %d, want exactly 1", wins)
	}
}
