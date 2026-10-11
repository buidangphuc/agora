package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/buidangphuc/team-payment/internal/pgtest"
)

// A paid or refunded payment can never be settled or failed again: the status
// write is a compare-and-set from PENDING/FAILED, so a mock payment that read the
// payment before a racing settle+refund cannot reopen it.
func checkSettleCAS(t *testing.T, repo PaymentRepository) {
	t.Helper()
	ctx := context.Background()
	for _, from := range []PaymentStatus{PaymentStatusPaid, PaymentStatusRefunded, PaymentStatusPartiallyRefunded} {
		tx, err := repo.CreateTransaction(ctx, PaymentTransaction{
			OrderID: fmt.Sprintf("order-cas-%d", from), BuyerID: "b", Amount: 500, Status: PaymentStatusPending,
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if _, err := repo.UpdateTransactionStatus(ctx, tx.ID, PaymentStatusPaid, "ref"); err != nil {
			t.Fatalf("settle from PENDING: %v", err)
		}
		if from != PaymentStatusPaid {
			forceStatus(t, repo, tx.ID, from)
		}
		for _, to := range []PaymentStatus{PaymentStatusPaid, PaymentStatusFailed} {
			if _, err := repo.UpdateTransactionStatus(ctx, tx.ID, to, "stale"); !errors.Is(err, ErrNotSettleable) {
				t.Fatalf("%v -> %v: %v, want ErrNotSettleable", from, to, err)
			}
		}
		got, _ := repo.GetTransaction(ctx, tx.ID)
		if got.Status != from {
			t.Fatalf("status %v, want %v unchanged", got.Status, from)
		}
	}
	if _, err := repo.UpdateTransactionStatus(ctx, "missing-tx", PaymentStatusPaid, ""); !errors.Is(err, ErrTransactionNotFound) {
		t.Fatalf("missing: %v, want ErrTransactionNotFound", err)
	}
}

func forceStatus(t *testing.T, repo PaymentRepository, id string, st PaymentStatus) {
	t.Helper()
	switch r := repo.(type) {
	case *InMemoryPaymentRepository:
		if _, err := r.UpdateStatus(context.Background(), id, st, "forced"); err != nil {
			t.Fatal(err)
		}
	case *PostgresPaymentRepository:
		// keep the 0007 status/refunded_amount checks satisfied
		const q = `UPDATE payment_transactions SET status = $1::int,
			refunded_amount = CASE $1::int WHEN 4 THEN amount WHEN 5 THEN amount / 2 ELSE refunded_amount END
			WHERE id = $2`
		if _, err := r.pool.Exec(context.Background(), q, int32(st), id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSettleIsCompareAndSet_InMemory(t *testing.T) {
	checkSettleCAS(t, NewInMemoryPaymentRepository())
}

func TestSettleIsCompareAndSet_Postgres(t *testing.T) {
	checkSettleCAS(t, NewPostgresPaymentRepository(pgtest.Pool(t)))
}
