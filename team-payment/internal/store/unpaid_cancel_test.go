package store_test

import (
	"context"
	"testing"

	"github.com/buidangphuc/team-payment/internal/store"
	"github.com/buidangphuc/team-payment/internal/store/fake"
)

type unpaidCanceller interface {
	store.PaymentRepository
	CancelUnpaid(ctx context.Context, orderID string) (store.CancelUnpaidOutcome, error)
}

func seedTx(t *testing.T, repo unpaidCanceller, orderID string, st store.PaymentStatus) store.PaymentTransaction {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.CreateTransaction(ctx, store.PaymentTransaction{
		OrderID: orderID, BuyerID: "b", Amount: 500, Status: store.PaymentStatusPending,
	})
	if err != nil {
		t.Fatal(err)
	}
	switch st {
	case store.PaymentStatusPending:
	case store.PaymentStatusFailed, store.PaymentStatusPaid:
		if tx, err = repo.UpdateTransactionStatus(ctx, tx.ID, st, "ref"); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("seed status %v unsupported", st)
	}
	return tx
}

func cancel(t *testing.T, repo unpaidCanceller, orderID string) store.CancelUnpaidOutcome {
	t.Helper()
	got, err := repo.CancelUnpaid(context.Background(), orderID)
	if err != nil {
		t.Fatalf("CancelUnpaid(%s): %v", orderID, err)
	}
	return got
}

func status(t *testing.T, repo unpaidCanceller, id string) store.PaymentStatus {
	t.Helper()
	tx, err := repo.GetTransaction(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return tx.Status
}

// Scenarios: Pending transaction is voided on unpaid cancel; Failed transaction is
// voided on unpaid cancel.
func checkCancelVoids(t *testing.T, repo unpaidCanceller, from store.PaymentStatus) {
	tx := seedTx(t, repo, "O-void", from)
	if got := cancel(t, repo, "O-void"); got != store.CancelVoided {
		t.Fatalf("outcome %v, want CancelVoided", got)
	}
	if s := status(t, repo, tx.ID); s != store.PaymentStatusCancelled {
		t.Fatalf("status %v, want CANCELLED", s)
	}
}

func TestCancelUnpaidVoidsPendingTransaction_InMemory(t *testing.T) {
	checkCancelVoids(t, fake.NewInMemoryPaymentRepository(), store.PaymentStatusPending)
}

func TestCancelUnpaidVoidsFailedTransaction_InMemory(t *testing.T) {
	checkCancelVoids(t, fake.NewInMemoryPaymentRepository(), store.PaymentStatusFailed)
}

func TestCancelUnpaidVoidsPendingTransaction_Postgres(t *testing.T) {
	pool := migratedPool(t)
	before := outboxCount(t, pool)
	checkCancelVoids(t, store.NewPostgresPaymentRepository(pool), store.PaymentStatusPending)
	if after := outboxCount(t, pool); after != before {
		t.Fatalf("outbox rows %d -> %d, void must write none", before, after)
	}
}

func TestCancelUnpaidVoidsFailedTransaction_Postgres(t *testing.T) {
	pool := migratedPool(t)
	before := outboxCount(t, pool)
	checkCancelVoids(t, store.NewPostgresPaymentRepository(pool), store.PaymentStatusFailed)
	if after := outboxCount(t, pool); after != before {
		t.Fatalf("outbox rows %d -> %d, void must write none", before, after)
	}
}

// Scenario: Order without a transaction is a no-op.
func checkCancelAbsent(t *testing.T, repo unpaidCanceller) {
	other := seedTx(t, repo, "O-other", store.PaymentStatusPending)
	if got := cancel(t, repo, "O-none"); got != store.CancelAbsent {
		t.Fatalf("outcome %v, want CancelAbsent", got)
	}
	if s := status(t, repo, other.ID); s != store.PaymentStatusPending {
		t.Fatalf("other order's status %v, want PENDING untouched", s)
	}
}

func TestCancelUnpaidWithoutTransactionIsNoop_InMemory(t *testing.T) {
	checkCancelAbsent(t, fake.NewInMemoryPaymentRepository())
}

func TestCancelUnpaidWithoutTransactionIsNoop_Postgres(t *testing.T) {
	checkCancelAbsent(t, store.NewPostgresPaymentRepository(migratedPool(t)))
}

// Scenario: Redelivered cancel leaves one end state.
func checkCancelRedelivered(t *testing.T, repo unpaidCanceller) {
	tx := seedTx(t, repo, "O-redeliver", store.PaymentStatusPending)
	if got := cancel(t, repo, "O-redeliver"); got != store.CancelVoided {
		t.Fatalf("first outcome %v, want CancelVoided", got)
	}
	if got := cancel(t, repo, "O-redeliver"); got != store.CancelAlreadyCancelled {
		t.Fatalf("second outcome %v, want CancelAlreadyCancelled", got)
	}
	if s := status(t, repo, tx.ID); s != store.PaymentStatusCancelled {
		t.Fatalf("status %v, want CANCELLED", s)
	}
}

func TestCancelUnpaidRedeliveredLeavesOneEndState_InMemory(t *testing.T) {
	checkCancelRedelivered(t, fake.NewInMemoryPaymentRepository())
}

func TestCancelUnpaidRedeliveredLeavesOneEndState_Postgres(t *testing.T) {
	checkCancelRedelivered(t, store.NewPostgresPaymentRepository(migratedPool(t)))
}

// Compare-and-set: a settled payment is never voided, and CANCELLED is terminal
// (a settle or failure write cannot reopen it).
func checkCancelCAS(t *testing.T, repo unpaidCanceller) {
	ctx := context.Background()
	paid := seedTx(t, repo, "O-paid", store.PaymentStatusPaid)
	if got := cancel(t, repo, "O-paid"); got != store.CancelOtherStatus {
		t.Fatalf("paid outcome %v, want CancelOtherStatus", got)
	}
	if s := status(t, repo, paid.ID); s != store.PaymentStatusPaid {
		t.Fatalf("paid status %v, want PAID unchanged", s)
	}
	voided := seedTx(t, repo, "O-term", store.PaymentStatusPending)
	cancel(t, repo, "O-term")
	for _, to := range []store.PaymentStatus{store.PaymentStatusPaid, store.PaymentStatusFailed} {
		if _, err := repo.UpdateTransactionStatus(ctx, voided.ID, to, "late"); err == nil {
			t.Fatalf("CANCELLED -> %v succeeded, want ErrNotSettleable", to)
		}
	}
	if s := status(t, repo, voided.ID); s != store.PaymentStatusCancelled {
		t.Fatalf("status %v, want CANCELLED unchanged", s)
	}
}

func TestCancelUnpaidIsCompareAndSet_InMemory(t *testing.T) {
	checkCancelCAS(t, fake.NewInMemoryPaymentRepository())
}

func TestCancelUnpaidIsCompareAndSet_Postgres(t *testing.T) {
	checkCancelCAS(t, store.NewPostgresPaymentRepository(migratedPool(t)))
}
