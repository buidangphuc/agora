package repository

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Concurrent AppendDebit calls for one seller must serialize on the advisory lock:
// with a 1000 balance and 20 racing debits of 100 exactly 10 succeed and the sum
// never goes negative (a read-then-insert would let more than 10 through). Runs
// only against a disposable Postgres (TEST_DATABASE_URL).
func TestAppendDebitConcurrent_Postgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("no TEST_DATABASE_URL set; skipping Postgres ledger concurrency test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("cannot connect to Postgres (%v); skipping", err)
	}
	// Registered first so it runs last: t.Cleanup is LIFO, and the reset cleanup
	// below still needs the pool (a defer would close it before cleanups run).
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("Postgres unreachable (%v); skipping", err)
	}
	ddl, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0004_wallet_ledger.up.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(ddl)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}

	const seller = "seller-debit-race"
	reset := func() {
		if _, err := pool.Exec(ctx, `DELETE FROM wallet_ledger WHERE seller_id = $1`, seller); err != nil {
			t.Fatalf("reset ledger: %v", err)
		}
	}
	reset()
	t.Cleanup(reset)

	repo := NewPostgresLedgerRepository(pool)
	if _, err := repo.AppendEntry(ctx, LedgerEntry{
		SellerID: seller, Type: LedgerTypeOrderSettlement, Amount: 1000, Status: LedgerStatusCompleted,
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
