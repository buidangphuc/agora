package repository_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-payment/internal/pgtest"
	"github.com/buidangphuc/team-payment/internal/repository"
)

type unpaidCanceller interface {
	repository.PaymentRepository
	CancelUnpaid(ctx context.Context, orderID string) (repository.CancelUnpaidOutcome, error)
}

func outboxCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM payment_outbox_events`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func seedTx(t *testing.T, repo unpaidCanceller, orderID string, st repository.PaymentStatus) repository.PaymentTransaction {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.CreateTransaction(ctx, repository.PaymentTransaction{
		OrderID: orderID, BuyerID: "b", Amount: 500, Status: repository.PaymentStatusPending,
	})
	if err != nil {
		t.Fatal(err)
	}
	switch st {
	case repository.PaymentStatusPending:
	case repository.PaymentStatusFailed, repository.PaymentStatusPaid:
		if tx, err = repo.UpdateTransactionStatus(ctx, tx.ID, st, "ref"); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("seed status %v unsupported", st)
	}
	return tx
}

func cancel(t *testing.T, repo unpaidCanceller, orderID string) repository.CancelUnpaidOutcome {
	t.Helper()
	got, err := repo.CancelUnpaid(context.Background(), orderID)
	if err != nil {
		t.Fatalf("CancelUnpaid(%s): %v", orderID, err)
	}
	return got
}

func status(t *testing.T, repo unpaidCanceller, id string) repository.PaymentStatus {
	t.Helper()
	tx, err := repo.GetTransaction(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return tx.Status
}

// Scenarios: Pending transaction is voided on unpaid cancel; Failed transaction is
// voided on unpaid cancel.
func checkCancelVoids(t *testing.T, repo unpaidCanceller, from repository.PaymentStatus) {
	tx := seedTx(t, repo, "O-void", from)
	if got := cancel(t, repo, "O-void"); got != repository.CancelVoided {
		t.Fatalf("outcome %v, want CancelVoided", got)
	}
	if s := status(t, repo, tx.ID); s != repository.PaymentStatusCancelled {
		t.Fatalf("status %v, want CANCELLED", s)
	}
}

func TestCancelUnpaidVoidsPendingTransaction_InMemory(t *testing.T) {
	checkCancelVoids(t, repository.NewInMemoryPaymentRepository(), repository.PaymentStatusPending)
}

func TestCancelUnpaidVoidsFailedTransaction_InMemory(t *testing.T) {
	checkCancelVoids(t, repository.NewInMemoryPaymentRepository(), repository.PaymentStatusFailed)
}

func TestCancelUnpaidVoidsPendingTransaction_Postgres(t *testing.T) {
	pool := pgtest.Pool(t)
	before := outboxCount(t, pool)
	checkCancelVoids(t, repository.NewPostgresPaymentRepository(pool), repository.PaymentStatusPending)
	if after := outboxCount(t, pool); after != before {
		t.Fatalf("outbox rows %d -> %d, void must write none", before, after)
	}
}

func TestCancelUnpaidVoidsFailedTransaction_Postgres(t *testing.T) {
	pool := pgtest.Pool(t)
	before := outboxCount(t, pool)
	checkCancelVoids(t, repository.NewPostgresPaymentRepository(pool), repository.PaymentStatusFailed)
	if after := outboxCount(t, pool); after != before {
		t.Fatalf("outbox rows %d -> %d, void must write none", before, after)
	}
}

// Scenario: Order without a transaction is a no-op.
func checkCancelAbsent(t *testing.T, repo unpaidCanceller) {
	other := seedTx(t, repo, "O-other", repository.PaymentStatusPending)
	if got := cancel(t, repo, "O-none"); got != repository.CancelAbsent {
		t.Fatalf("outcome %v, want CancelAbsent", got)
	}
	if s := status(t, repo, other.ID); s != repository.PaymentStatusPending {
		t.Fatalf("other order's status %v, want PENDING untouched", s)
	}
}

func TestCancelUnpaidWithoutTransactionIsNoop_InMemory(t *testing.T) {
	checkCancelAbsent(t, repository.NewInMemoryPaymentRepository())
}

func TestCancelUnpaidWithoutTransactionIsNoop_Postgres(t *testing.T) {
	checkCancelAbsent(t, repository.NewPostgresPaymentRepository(pgtest.Pool(t)))
}

// Scenario: Redelivered cancel leaves one end state.
func checkCancelRedelivered(t *testing.T, repo unpaidCanceller) {
	tx := seedTx(t, repo, "O-redeliver", repository.PaymentStatusPending)
	if got := cancel(t, repo, "O-redeliver"); got != repository.CancelVoided {
		t.Fatalf("first outcome %v, want CancelVoided", got)
	}
	if got := cancel(t, repo, "O-redeliver"); got != repository.CancelAlreadyCancelled {
		t.Fatalf("second outcome %v, want CancelAlreadyCancelled", got)
	}
	if s := status(t, repo, tx.ID); s != repository.PaymentStatusCancelled {
		t.Fatalf("status %v, want CANCELLED", s)
	}
}

func TestCancelUnpaidRedeliveredLeavesOneEndState_InMemory(t *testing.T) {
	checkCancelRedelivered(t, repository.NewInMemoryPaymentRepository())
}

func TestCancelUnpaidRedeliveredLeavesOneEndState_Postgres(t *testing.T) {
	checkCancelRedelivered(t, repository.NewPostgresPaymentRepository(pgtest.Pool(t)))
}

// Compare-and-set: a settled payment is never voided, and CANCELLED is terminal
// (a settle or failure write cannot reopen it).
func checkCancelCAS(t *testing.T, repo unpaidCanceller) {
	ctx := context.Background()
	paid := seedTx(t, repo, "O-paid", repository.PaymentStatusPaid)
	if got := cancel(t, repo, "O-paid"); got != repository.CancelOtherStatus {
		t.Fatalf("paid outcome %v, want CancelOtherStatus", got)
	}
	if s := status(t, repo, paid.ID); s != repository.PaymentStatusPaid {
		t.Fatalf("paid status %v, want PAID unchanged", s)
	}
	voided := seedTx(t, repo, "O-term", repository.PaymentStatusPending)
	cancel(t, repo, "O-term")
	for _, to := range []repository.PaymentStatus{repository.PaymentStatusPaid, repository.PaymentStatusFailed} {
		if _, err := repo.UpdateTransactionStatus(ctx, voided.ID, to, "late"); err == nil {
			t.Fatalf("CANCELLED -> %v succeeded, want ErrNotSettleable", to)
		}
	}
	if s := status(t, repo, voided.ID); s != repository.PaymentStatusCancelled {
		t.Fatalf("status %v, want CANCELLED unchanged", s)
	}
}

func TestCancelUnpaidIsCompareAndSet_InMemory(t *testing.T) {
	checkCancelCAS(t, repository.NewInMemoryPaymentRepository())
}

func TestCancelUnpaidIsCompareAndSet_Postgres(t *testing.T) {
	checkCancelCAS(t, repository.NewPostgresPaymentRepository(pgtest.Pool(t)))
}
