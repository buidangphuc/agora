package consumer_test

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/buidangphuc/team-payment/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	"github.com/buidangphuc/team-payment/internal/consumer"
	"github.com/buidangphuc/team-payment/internal/repository"
)

// returnRecord is team-order's ReturnRefunded fact as its outbox writes it: keyed by the
// order id, event id stable per return.
func returnRecord(t *testing.T, orderID, returnID string, amount int64) consumer.Record {
	t.Helper()
	return consumer.Record{Key: orderID + "/" + returnID, Value: envelope(t, consumer.ReturnRefundedEventType, "rr-"+returnID,
		&orderv1.ReturnRefunded{ReturnId: returnID, OrderId: orderID, BuyerId: "buyer", SellerId: "s1",
			RefundAmount: amount, Currency: "VND", RefundedAt: timestamppb.Now()})}
}

func (f fixture) refunds(t *testing.T, txID string) []repository.Refund {
	t.Helper()
	tx, err := f.svc.GetPayment(context.Background(), txID, "")
	if err != nil {
		t.Fatal(err)
	}
	return tx.Refunds
}

func TestReturnRefunded_RefundsOnceAndRedeliveryIsNoop(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		tx := f.paid(t, "o1", 500000)
		rr := returnRecord(t, "o1", "ret-1", 200000)
		dlq := consume(t, f, paidRecord(t, "o1", 500000, "s1"), rr, rr, paidRecord(t, "o1", 500000, "s1"), rr)
		got := f.status(t, tx.ID)
		if got.Status != repository.PaymentStatusPartiallyRefunded || got.RefundedAmount != 200000 {
			t.Fatalf("payment: %+v", got)
		}
		refunds := f.refunds(t, tx.ID)
		if len(refunds) != 1 || refunds[0].ID != "return:ret-1" || refunds[0].Source != repository.RefundSourceReturn ||
			refunds[0].SourceID != "ret-1" || refunds[0].Reason != "return_refunded" || refunds[0].Amount != 200000 {
			t.Fatalf("refunds: %+v", refunds)
		}
		f.wantLedger(t, "s1", tx.ID, 500000, map[string]int64{"return:ret-1": -200000})
		if len(dlq.parked()) != 0 {
			t.Fatalf("dlq %v", dlq.parked())
		}
	})
}

func TestReturnRefunded_BeforeCreditEndsWithOneDeduction(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		tx := f.paid(t, "o1", 500000)
		consume(t, f, returnRecord(t, "o1", "ret-1", 200000), paidRecord(t, "o1", 500000, "s1"))
		f.wantLedger(t, "s1", tx.ID, 500000, map[string]int64{"return:ret-1": -200000})
	})
}

func TestReturnRefunded_TwoReturnsRefundCumulatively(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		tx := f.paid(t, "o1", 500000)
		consume(t, f, paidRecord(t, "o1", 500000, "s1"), returnRecord(t, "o1", "ret-1", 200000), returnRecord(t, "o1", "ret-2", 300000))
		if got := f.status(t, tx.ID); got.Status != repository.PaymentStatusRefunded || got.RefundedAmount != 500000 {
			t.Fatalf("payment: %+v", got)
		}
		f.wantLedger(t, "s1", tx.ID, 500000, map[string]int64{"return:ret-1": -200000, "return:ret-2": -300000})
	})
}

// A direct refund took part of the money: the return refunds only the remainder, and
// with nothing left it records an applied 0 without a deduction or a DLQ record.
func TestReturnRefunded_ClampsAfterADirectRefund(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		tx := f.paid(t, "o1", 500000)
		consume(t, f, paidRecord(t, "o1", 500000, "s1"))
		if _, _, _, err := f.svc.RefundPayment(context.Background(), tx.ID, "D", 400000, "seller"); err != nil {
			t.Fatal(err)
		}
		dlq := consume(t, f, returnRecord(t, "o1", "ret-1", 300000), returnRecord(t, "o1", "ret-2", 200000))
		if got := f.status(t, tx.ID); got.Status != repository.PaymentStatusRefunded || got.RefundedAmount != 500000 {
			t.Fatalf("payment: %+v", got)
		}
		refunds := f.refunds(t, tx.ID)
		if len(refunds) != 3 || refunds[1].RequestedAmount != 300000 || refunds[1].Amount != 100000 ||
			refunds[2].RequestedAmount != 200000 || refunds[2].Amount != 0 {
			t.Fatalf("refunds: %+v", refunds)
		}
		f.wantLedger(t, "s1", tx.ID, 500000, map[string]int64{"rpc:D": -400000, "return:ret-1": -100000})
		if len(dlq.parked()) != 0 {
			t.Fatalf("an applied 0 must not be dead-lettered: %v", dlq.parked())
		}
	})
}

// A return refund and a cancel refund the payment once in total.
func TestReturnRefunded_ThenCancelRefundsTheRemainder(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		tx := f.paid(t, "o1", 500000)
		consume(t, f, paidRecord(t, "o1", 500000, "s1"), returnRecord(t, "o1", "ret-1", 200000),
			cancelledRecord(t, "o1", orderv1.OrderStatus_ORDER_STATUS_PAID, 500000))
		if got := f.status(t, tx.ID); got.Status != repository.PaymentStatusRefunded || got.RefundedAmount != 500000 {
			t.Fatalf("payment: %+v", got)
		}
		f.wantLedger(t, "s1", tx.ID, 500000, map[string]int64{"return:ret-1": -200000, "cancel:o1": -300000})
	})
}

func TestReturnRefunded_UnapplicableFactsAreDeadLettered(t *testing.T) {
	eachBackend(t, func(t *testing.T, f fixture) {
		f.paid(t, "o-ok", 100)
		badPayload := consumer.Record{Key: "bad-payload", Value: mustMarshal(t, &eventsv1.EventEnvelope{EventId: "e", Type: consumer.ReturnRefundedEventType, Payload: []byte{0xff, 0xff}})}
		noReturn := returnRecord(t, "o-ok", "", 10)
		noReturn.Key = "no-return"
		noOrder := returnRecord(t, "", "ret-x", 10)
		noOrder.Key = "no-order"
		zero := returnRecord(t, "o-ok", "ret-zero", 0)
		zero.Key = "zero"
		negative := returnRecord(t, "o-ok", "ret-neg", -5)
		negative.Key = "negative"
		noPayment := returnRecord(t, "o-no-payment", "ret-np", 100)
		noPayment.Key = "no-payment"
		dlq := consume(t, f, badPayload, noReturn, noOrder, zero, negative, noPayment, paidRecord(t, "o-ok", 100, "s1"))
		want := []string{"bad-payload", "no-return", "no-order", "zero", "negative", "no-payment"}
		got := dlq.parked()
		if len(got) != len(want) {
			t.Fatalf("dlq %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("dlq %v, want %v", got, want)
			}
		}
		if c := f.entries(t, "s1")[repository.LedgerTypeOrderSettlement]; len(c) != 1 {
			t.Fatalf("the later paid order must be credited once: %+v", c)
		}
		if d := f.entries(t, "s1")[repository.LedgerTypeRefundDeduction]; len(d) != 0 {
			t.Fatalf("a parked fact wrote a deduction: %+v", d)
		}
	})
}

func TestReturnRefunded_TransientErrorRetriedThenDLQ(t *testing.T) {
	a := &failingApplier{}
	reader := &fakeReader{records: []consumer.Record{returnRecord(t, "o1", "ret-1", 100)}}
	dlq := &fakeDLQ{}
	run(t, a, reader, dlq, consumer.RunConfig{MaxAttempts: 5, BaseBackoff: time.Millisecond}, committedN(reader, 1))
	if a.calls.Load() != 5 || len(dlq.parked()) != 1 {
		t.Fatalf("calls=%d dlq=%v, want 5 attempts then DLQ", a.calls.Load(), dlq.parked())
	}
}
