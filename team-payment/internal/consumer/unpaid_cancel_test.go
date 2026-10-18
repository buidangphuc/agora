package consumer_test

import (
	"context"
	"testing"

	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	"github.com/buidangphuc/team-payment/internal/consumer"
	"github.com/buidangphuc/team-payment/internal/repository"
)

const (
	stPending = orderv1.OrderStatus_ORDER_STATUS_PENDING
	stPaid    = orderv1.OrderStatus_ORDER_STATUS_PAID
)

func (f fixture) open(t *testing.T, orderID string, amount int64, status repository.PaymentStatus) repository.PaymentTransaction {
	t.Helper()
	ctx := context.Background()
	tx, err := f.payments.CreateTransaction(ctx, repository.PaymentTransaction{OrderID: orderID, BuyerID: "buyer", Amount: amount})
	if err != nil {
		t.Fatal(err)
	}
	if status != repository.PaymentStatusPending {
		if tx, err = f.payments.UpdateTransactionStatus(ctx, tx.ID, status, "MOCK"); err != nil {
			t.Fatal(err)
		}
	}
	return tx
}

// refundRows returns the payment's refund rows as read through GetPayment.
func (f fixture) refundRows(t *testing.T, txID string) []repository.Refund {
	t.Helper()
	tx, err := f.svc.GetPayment(context.Background(), txID, "")
	if err != nil {
		t.Fatal(err)
	}
	return tx.Refunds
}

func (f fixture) wantRefund(t *testing.T, txID, key string, amount int64) {
	t.Helper()
	rs := f.refundRows(t, txID)
	if len(rs) != 1 || rs[0].ID != key || rs[0].Amount != amount {
		t.Fatalf("refund rows %+v, want one %s of %d", rs, key, amount)
	}
}

func wantNoDLQ(t *testing.T, dlq *fakeDLQ) {
	t.Helper()
	if got := dlq.parked(); len(got) != 0 {
		t.Fatalf("dlq %v", got)
	}
}

// Scenario: Pending transaction is voided on unpaid cancel
func TestCancelledPendingVoidsPendingTransaction(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		tx := f.open(t, "O-1", 500000, repository.PaymentStatusPending)
		dlq := consume(t, f, cancelledRecord(t, "O-1", stPending, 500000))
		if got := f.status(t, tx.ID); got.Status != repository.PaymentStatusCancelled || got.RefundedAmount != 0 {
			t.Fatalf("payment: %+v", got)
		}
		if n := len(f.refundRows(t, tx.ID)); n != 0 {
			t.Fatalf("%d refund rows", n)
		}
		wantNoDLQ(t, dlq)
	})
}

// Scenario: Failed transaction is voided on unpaid cancel
func TestCancelledPendingVoidsFailedTransaction(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		tx := f.open(t, "O-2", 500000, repository.PaymentStatusFailed)
		dlq := consume(t, f, cancelledRecord(t, "O-2", stPending, 500000))
		if got := f.status(t, tx.ID); got.Status != repository.PaymentStatusCancelled {
			t.Fatalf("payment: %+v", got)
		}
		wantNoDLQ(t, dlq)
	})
}

// Scenario: Order without a transaction is a no-op
func TestCancelledPendingWithoutTransactionIsNoop(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		rec := cancelledRecord(t, "O-cod", stPending, 1)
		reader := &fakeReader{records: []consumer.Record{rec}}
		dlq := &fakeDLQ{}
		run(t, f.svc, reader, dlq, slow, committedN(reader, 1))
		wantNoDLQ(t, dlq)
		if _, err := f.payments.GetTransactionByOrderID(context.Background(), "O-cod"); err == nil {
			t.Fatal("a transaction was created")
		}
	})
}

// Scenario: Redelivered cancel leaves one end state
func TestCancelledPendingRedeliveryIsIdempotent(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		tx := f.open(t, "O-1", 500000, repository.PaymentStatusPending)
		c := cancelledRecord(t, "O-1", stPending, 500000)
		dlq := consume(t, f, c, c)
		if got := f.status(t, tx.ID); got.Status != repository.PaymentStatusCancelled {
			t.Fatalf("payment: %+v", got)
		}
		if n := len(f.refundRows(t, tx.ID)); n != 0 {
			t.Fatalf("%d refund rows", n)
		}
		wantNoDLQ(t, dlq)
	})
}

// Scenario: Shipped-order cancel changes no transaction
func TestCancelledShippedLeavesTransactionUnchanged(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		tx := f.paid(t, "O-4", 500000)
		dlq := consume(t, f, cancelledRecord(t, "O-4", orderv1.OrderStatus_ORDER_STATUS_SHIPPED, 500000))
		if got := f.status(t, tx.ID); got.Status != repository.PaymentStatusPaid || got.RefundedAmount != 0 {
			t.Fatalf("payment: %+v", got)
		}
		if n := len(f.refundRows(t, tx.ID)); n != 0 {
			t.Fatalf("%d refund rows", n)
		}
		wantNoDLQ(t, dlq)
	})
}

// Scenario: Paid-order cancel still refunds without voiding
func TestCancelledPaidStillRefundsWithoutVoid(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		tx := f.paid(t, "O-5", 300000)
		consume(t, f, paidRecord(t, "O-5", 300000, "s1"), cancelledRecord(t, "O-5", stPaid, 300000))
		if got := f.status(t, tx.ID); got.Status != repository.PaymentStatusRefunded {
			t.Fatalf("payment: %+v", got)
		}
		f.wantRefund(t, tx.ID, "cancel:O-5", 300000)
	})
}

// Scenario: Malformed cancel is parked
func TestCancelledMissingOrderIDIsPermanent(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		rec := cancelledRecord(t, "", stPending, 1)
		rec.Key = "malformed"
		reader := &fakeReader{records: []consumer.Record{rec}}
		dlq := &fakeDLQ{}
		run(t, f.svc, reader, dlq, slow, committedN(reader, 1))
		if got := dlq.parked(); len(got) != 1 || got[0] != "malformed" {
			t.Fatalf("dlq %v", got)
		}
	})
}

// Scenario: Late payment is refunded in full
func TestCancelledPendingRefundsLateSettledPayment(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		tx := f.paid(t, "O-3", 500000)
		dlq := consume(t, f, cancelledRecord(t, "O-3", stPending, 500000))
		if got := f.status(t, tx.ID); got.Status != repository.PaymentStatusRefunded || got.RefundedAmount != 500000 {
			t.Fatalf("payment: %+v", got)
		}
		f.wantRefund(t, tx.ID, "cancel:O-3", 500000)
		if e := f.entries(t, "s1"); len(e) != 0 {
			t.Fatalf("ledger entries %+v", e)
		}
		wantNoDLQ(t, dlq)
	})
}

// Scenario: Redelivery after a late-payment refund moves no money again
func TestCancelledPendingLateRefundRedeliveryIsNoop(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		tx := f.paid(t, "O-3", 500000)
		c := cancelledRecord(t, "O-3", stPending, 500000)
		dlq := consume(t, f, c, c)
		if got := f.status(t, tx.ID); got.RefundedAmount != 500000 || got.Status != repository.PaymentStatusRefunded {
			t.Fatalf("payment: %+v", got)
		}
		f.wantRefund(t, tx.ID, "cancel:O-3", 500000)
		wantNoDLQ(t, dlq)
	})
}

// Scenario: Partially refunded late payment is refunded the remainder once
func TestCancelledPendingPartiallyRefundedRefundsRemainderOnce(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		tx := f.paid(t, "O-6", 500000)
		if _, _, _, err := f.svc.RefundPayment(context.Background(), tx.ID, "R1", 200000, "seller"); err != nil {
			t.Fatal(err)
		}
		c := cancelledRecord(t, "O-6", stPending, 500000)
		dlq := consume(t, f, c, c)
		if got := f.status(t, tx.ID); got.Status != repository.PaymentStatusRefunded || got.RefundedAmount != 500000 {
			t.Fatalf("payment: %+v", got)
		}
		var n int
		for _, r := range f.refundRows(t, tx.ID) {
			if r.ID == "cancel:O-6" {
				n++
				if r.Amount != 300000 {
					t.Fatalf("remainder refund %d", r.Amount)
				}
			}
		}
		if n != 1 {
			t.Fatalf("%d cancel refund rows", n)
		}
		wantNoDLQ(t, dlq)
	})
}
