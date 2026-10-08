package repository

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/buidangphuc/team-payment/internal/pgtest"
)

// Concurrent AppendDebit calls for one seller must serialize on the advisory lock:
// with a 1000 balance and 20 racing debits of 100 exactly 10 succeed and the sum
// never goes negative (a read-then-insert would let more than 10 through). Runs
// only against a disposable Postgres (TEST_DATABASE_URL).
func TestAppendDebitConcurrent_Postgres(t *testing.T) {
	pool := pgtest.Pool(t)
	ctx := context.Background()
	const seller = "seller-debit-race"

	repo := NewPostgresLedgerRepository(pool)
	if _, err := repo.AppendEntry(ctx, LedgerEntry{
		SellerID: seller, Type: LedgerTypeOrderSettlement, Amount: 1000, Status: LedgerStatusCompleted, ReferenceID: "tx-seed",
	}); err != nil {
		t.Fatalf("seed credit: %v", err)
	}

	const workers = 20
	var ok, insufficient atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := repo.AppendDebit(ctx, LedgerEntry{
				SellerID: seller, Type: LedgerTypePayout, Amount: -100, Status: LedgerStatusPending,
			})
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrInsufficientBalance):
				insufficient.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if ok.Load() != 10 || insufficient.Load() != 10 {
		t.Fatalf("ok=%d insufficient=%d, want 10/10", ok.Load(), insufficient.Load())
	}
	balance, err := repo.Balance(ctx, seller)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if balance != 0 {
		t.Fatalf("balance = %d, want 0", balance)
	}
}
