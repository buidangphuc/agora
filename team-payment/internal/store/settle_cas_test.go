package store_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-payment/internal/store"
	"github.com/buidangphuc/team-payment/internal/store/fake"
)

// A paid or refunded payment can never be settled or failed again: the status
// write is a compare-and-set from PENDING/FAILED, so a mock payment that read the
// payment before a racing settle+refund cannot reopen it.
func checkSettleCAS(t *testing.T, repo store.PaymentRepository, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	for _, from := range []store.PaymentStatus{store.PaymentStatusPaid, store.PaymentStatusRefunded, store.PaymentStatusPartiallyRefunded} {
		tx, err := repo.CreateTransaction(ctx, store.PaymentTransaction{
			OrderID: fmt.Sprintf("order-cas-%d", from), BuyerID: "b", Amount: 500, Status: store.PaymentStatusPending,
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if _, err := repo.UpdateTransactionStatus(ctx, tx.ID, store.PaymentStatusPaid, "ref"); err != nil {
			t.Fatalf("settle from PENDING: %v", err)
		}
		if from != store.PaymentStatusPaid {
			forceStatus(t, repo, pool, tx.ID, from)
		}
		for _, to := range []store.PaymentStatus{store.PaymentStatusPaid, store.PaymentStatusFailed} {
			if _, err := repo.UpdateTransactionStatus(ctx, tx.ID, to, "stale"); !errors.Is(err, store.ErrNotSettleable) {
				t.Fatalf("%v -> %v: %v, want store.ErrNotSettleable", from, to, err)
			}
		}
		got, _ := repo.GetTransaction(ctx, tx.ID)
		if got.Status != from {
			t.Fatalf("status %v, want %v unchanged", got.Status, from)
		}
	}
	if _, err := repo.UpdateTransactionStatus(ctx, "missing-tx", store.PaymentStatusPaid, ""); !errors.Is(err, store.ErrTransactionNotFound) {
		t.Fatalf("missing: %v, want store.ErrTransactionNotFound", err)
	}
}

func forceStatus(t *testing.T, repo store.PaymentRepository, pool *pgxpool.Pool, id string, st store.PaymentStatus) {
	t.Helper()
	switch r := repo.(type) {
	case *fake.InMemoryPaymentRepository:
		if _, err := r.UpdateStatus(context.Background(), id, st, "forced"); err != nil {
			t.Fatal(err)
		}
	case *store.PostgresPaymentRepository:
		_ = r
		// keep the 0007 status/refunded_amount checks satisfied
		const q = `UPDATE payment_transactions SET status = $1::int,
			refunded_amount = CASE $1::int WHEN 4 THEN amount WHEN 5 THEN amount / 2 ELSE refunded_amount END
			WHERE id = $2`
		if _, err := pool.Exec(context.Background(), q, int32(st), id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSettleIsCompareAndSet_InMemory(t *testing.T) {
	checkSettleCAS(t, fake.NewInMemoryPaymentRepository(), nil)
}

func TestSettleIsCompareAndSet_Postgres(t *testing.T) {
	pool := migratedPool(t)
	checkSettleCAS(t, store.NewPostgresPaymentRepository(pool), pool)
}
