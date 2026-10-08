package consumer_test

import (
	"context"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"

	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	"github.com/buidangphuc/team-payment/internal/consumer"
	"github.com/buidangphuc/team-payment/internal/repository"
)

func cancelledRecord(t *testing.T, orderID string, from orderv1.OrderStatus, total int64) consumer.Record {
	t.Helper()
	return consumer.Record{Key: orderID + "-cancel", Value: envelope(t, consumer.OrderCancelledEventType, "cancel-"+orderID,
		&orderv1.OrderCancelled{OrderId: orderID, BuyerId: "buyer", SellerId: "s1", PreviousStatus: from,
			TotalAmount: total, Currency: "VND", CancelledAt: timestamppb.Now()})}
}

func (f fixture) status(t *testing.T, txID string) repository.PaymentTransaction {
	t.Helper()
	tx, err := f.payments.GetTransaction(context.Background(), txID)
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

// wantLedger asserts the seller's settlement credits and refund deductions for one payment.
func (f fixture) wantLedger(t *testing.T, seller, txID string, credit, deduction int64) {
	t.Helper()
	got := f.entries(t, seller)
	check := func(typ string, want int64) {
		var n int
		for _, e := range got[typ] {
			if e.ReferenceID == txID {
				n++
				if e.Amount != want {
					t.Fatalf("%s amount %d, want %d", typ, e.Amount, want)
				}
			}
		}
		if (want != 0 && n != 1) || (want == 0 && n != 0) {
			t.Fatalf("%s entries for %s: %d (want amount %d): %+v", typ, txID, n, want, got)
		}
	}
	check(repository.LedgerTypeOrderSettlement, credit)
	check(repository.LedgerTypeRefundDeduction, deduction)
}

func consume(t *testing.T, f fixture, recs ...consumer.Record) *fakeDLQ {
	t.Helper()
	reader := &fakeReader{records: recs}
	dlq := &fakeDLQ{}
	run(t, f.svc, reader, dlq, slow, committedN(reader, len(recs)))
	return dlq
}

func TestCancel_AfterCreditRefundsAndDeductsFullAmount(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		tx := f.paid(t, "o1", 500000)
		dlq := consume(t, f, paidRecord(t, "o1", 500000, "s1"), cancelledRecord(t, "o1", orderv1.OrderStatus_ORDER_STATUS_PAID, 500000))
		if got := f.status(t, tx.ID); got.Status != repository.PaymentStatusRefunded || got.RefundedAmount != 500000 ||
			got.ProviderReference != "REFUND:order_cancelled" {
			t.Fatalf("payment: %+v", got)
		}
		f.wantLedger(t, "s1", tx.ID, 500000, -500000)
		if bal, _ := f.ledger.Balance(context.Background(), "s1"); bal != 0 || len(dlq.parked()) != 0 {
			t.Fatalf("balance %d dlq %v", bal, dlq.parked())
		}
	})
}

func TestCancel_BeforeCreditEndsWithOneCreditOneDeduction(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		tx := f.paid(t, "o1", 500000)
		consume(t, f, cancelledRecord(t, "o1", orderv1.OrderStatus_ORDER_STATUS_PAID, 500000), paidRecord(t, "o1", 500000, "s1"))
		if f.status(t, tx.ID).Status != repository.PaymentStatusRefunded {
			t.Fatal("payment not refunded")
		}
		f.wantLedger(t, "s1", tx.ID, 500000, -500000)
	})
}

func TestCancel_RedeliveredIsNoop(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		tx := f.paid(t, "o1", 500000)
		c := cancelledRecord(t, "o1", orderv1.OrderStatus_ORDER_STATUS_PAID, 500000)
		dlq := consume(t, f, paidRecord(t, "o1", 500000, "s1"), c, c, paidRecord(t, "o1", 500000, "s1"), c)
		f.wantLedger(t, "s1", tx.ID, 500000, -500000)
		if len(dlq.parked()) != 0 {
			t.Fatalf("dlq %v", dlq.parked())
		}
	})
}

func TestCancel_AfterSellerPartialRefundDeductsOnlyThePartialAmount(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		tx := f.paid(t, "o1", 500000)
		consume(t, f, paidRecord(t, "o1", 500000, "s1"))
		if _, _, _, err := f.svc.RefundPayment(context.Background(), tx.ID, 200000, "seller"); err != nil {
			t.Fatal(err)
		}
		consume(t, f, cancelledRecord(t, "o1", orderv1.OrderStatus_ORDER_STATUS_PAID, 500000))
		if got := f.status(t, tx.ID); got.Status != repository.PaymentStatusRefunded || got.RefundedAmount != 200000 {
			t.Fatalf("payment: %+v", got)
		}
		f.wantLedger(t, "s1", tx.ID, 500000, -200000)
	})
}

func TestCancel_FromPendingWritesNothing(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		// A late payment for an order cancelled while Pending: PAID, but never credited
		// and not refunded by the cancel.
		tx := f.paid(t, "o1", 500000)
		dlq := consume(t, f, cancelledRecord(t, "o1", orderv1.OrderStatus_ORDER_STATUS_PENDING, 500000))
		if got := f.status(t, tx.ID); got.Status != repository.PaymentStatusPaid || got.RefundedAmount != 0 {
			t.Fatalf("payment changed: %+v", got)
		}
		if len(f.entries(t, "s1")) != 0 || len(dlq.parked()) != 0 {
			t.Fatalf("ledger %+v dlq %v", f.entries(t, "s1"), dlq.parked())
		}
	})
}

func TestCancel_WithoutSettledTransactionIsDeadLettered(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		dlq := consume(t, f, cancelledRecord(t, "o-unknown", orderv1.OrderStatus_ORDER_STATUS_PAID, 1))
		if got := dlq.parked(); len(got) != 1 || got[0] != "o-unknown-cancel" {
			t.Fatalf("dlq %v", got)
		}
	})
}
