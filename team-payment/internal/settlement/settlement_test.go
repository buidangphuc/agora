package settlement_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	eventsv1 "github.com/buidangphuc/team-payment/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	"github.com/buidangphuc/team-payment/internal/settlement"
	"github.com/buidangphuc/team-payment/internal/store"
)

func TestConsumer_CreditsOnceAndRedeliveryIsNoop(t *testing.T) {
	f := newFixture(t)
	tx := f.paid(t, "o1", 500000)
	dlq := feed(t, f, paidRecord(t, "o1", 500000, "s1", "s1"), paidRecord(t, "o1", 500000, "s1", "s1"))
	got := f.entries(t, "s1")[store.LedgerTypeOrderSettlement]
	if len(got) != 1 || got[0].Amount != 500000 || got[0].ReferenceID != tx.ID || got[0].Status != store.LedgerStatusCompleted {
		t.Fatalf("credits: %+v", got)
	}
	if bal, err := f.ledger.Balance(context.Background(), "s1"); err != nil || bal != 500000 {
		t.Fatalf("balance %d err %v, want 500000", bal, err)
	}
	wantNoDLQ(t, dlq)
}

func TestConsumer_CreditsTheTransactionAmount(t *testing.T) {
	f := newFixture(t)
	f.paid(t, "o1", 400000)
	feed(t, f, paidRecord(t, "o1", 999, "s1"))
	if got := f.entries(t, "s1")[store.LedgerTypeOrderSettlement]; len(got) != 1 || got[0].Amount != 400000 {
		t.Fatalf("credits: %+v", got)
	}
}

func TestConsumer_IgnoresOrderShipped(t *testing.T) {
	f := newFixture(t)
	f.paid(t, "o1", 100)
	dlq := feed(t, f, shippedRecord(t, "o1"))
	if len(f.entries(t, "s1")) != 0 || len(dlq.parked()) != 0 {
		t.Fatalf("OrderShipped must be ignored: ledger %+v dlq %v", f.entries(t, "s1"), dlq.parked())
	}
	if n := dlq.loop.attemptsOf("o1-shipped"); n != 1 {
		t.Fatalf("OrderShipped attempts = %d, want 1", n)
	}
}

func TestConsumer_MultiSellerOrderIsPermanent(t *testing.T) {
	f := newFixture(t)
	f.paid(t, "o-mixed", 100)
	f.paid(t, "o-no-seller", 100)
	dlq := feed(t, f, paidRecord(t, "o-mixed", 100, "s1", "s2"), paidRecord(t, "o-no-seller", 100))
	if got := dlq.parked(); len(got) != 2 || got[0] != "o-mixed" || got[1] != "o-no-seller" {
		t.Fatalf("dlq %v, want [o-mixed o-no-seller]", got)
	}
	for _, key := range []string{"o-mixed", "o-no-seller"} {
		if n := dlq.loop.attemptsOf(key); n != 1 {
			t.Fatalf("%s attempts = %d, want 1", key, n)
		}
	}
	for _, s := range []string{"s1", "s2"} {
		if len(f.entries(t, s)) != 0 {
			t.Fatalf("seller %s was credited", s)
		}
	}
}

func TestConsumer_PoisonRecordsGoToDLQAfterOneAttempt(t *testing.T) {
	f := newFixture(t)
	f.paid(t, "o-ok", 100)
	f.paid(t, "o-mixed", 100)
	noEventID := rec{Key: "no-event-id", Value: envelope(t, settlement.OrderPaidEventType, "", &orderv1.OrderPaidEvent{OrderId: "o-ok"})}
	badPayload := rec{Key: "bad-payload", Value: mustMarshal(t, &eventsv1.EventEnvelope{EventId: "e", Type: settlement.OrderPaidEventType, Payload: []byte{0xff, 0xff}})}
	malformed := rec{Key: "malformed", Value: []byte("not an envelope \xff\xfe")}
	dlq := feed(t, f,
		malformed,
		noEventID,
		badPayload,
		paidRecord(t, "o-no-tx", 100, "s1"),
		paidRecord(t, "o-mixed", 100, "s1", "s2"),
		paidRecord(t, "o-no-seller", 100),
		paidRecord(t, "o-ok", 100, "s1"),
	)
	want := []string{"malformed", "no-event-id", "bad-payload", "o-no-tx", "o-mixed", "o-no-seller"}
	got := dlq.parked()
	if len(got) != len(want) {
		t.Fatalf("dlq %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dlq %v, want %v", got, want)
		}
		if n := dlq.loop.attemptsOf(want[i]); n != 1 {
			t.Fatalf("%s attempts = %d, want 1", want[i], n)
		}
	}
	for _, r := range dlq.dlq {
		if r.Topic != "order.events.payment-settlement.dlq" {
			t.Fatalf("dlq topic %q, want order.events.payment-settlement.dlq", r.Topic)
		}
	}
	if string(dlq.dlq[0].Value) != string(malformed.Value) {
		t.Fatalf("dead-lettered value %q, want the original bytes", dlq.dlq[0].Value)
	}
	if got := f.entries(t, "s1")[store.LedgerTypeOrderSettlement]; len(got) != 1 || got[0].Amount != 100 {
		t.Fatalf("the paid order after the poison records must be credited once: %+v", got)
	}
	if len(f.entries(t, "s2")) != 0 {
		t.Fatal("a mixed-seller order credited a seller")
	}
}

type failingApplier struct{ calls atomic.Int32 }

func (a *failingApplier) CreditSettlement(context.Context, string, string, int64) error {
	a.calls.Add(1)
	return errors.New("connection refused")
}

func (a *failingApplier) RefundCancelledOrder(context.Context, string) error {
	a.calls.Add(1)
	return errors.New("connection refused")
}

func (a *failingApplier) RefundReturn(context.Context, string, string, int64) error {
	a.calls.Add(1)
	return errors.New("connection refused")
}

func TestConsumer_TransientErrorRetriedThenDLQ(t *testing.T) {
	a := &failingApplier{}
	o := feedHandler(t, settlement.NewHandler(a, quiet), paidRecord(t, "o1", 100, "s1"))
	if a.calls.Load() != 5 || len(o.dlq) != 1 || string(o.dlq[0].Key) != "o1" {
		t.Fatalf("calls=%d dlq=%v, want 5 attempts then DLQ", a.calls.Load(), o.parked())
	}
	if n := o.loop.attemptsOf("o1"); n != 5 {
		t.Fatalf("handler attempts = %d, want 5", n)
	}
	if n := o.loop.attemptsOf(sentinelKey); n != 1 {
		t.Fatalf("the next record must be processed once, got %d", n)
	}
}

func TestReturnRefunded_TransientErrorRetriedThenDLQ(t *testing.T) {
	a := &failingApplier{}
	o := feedHandler(t, settlement.NewHandler(a, quiet), returnRecord(t, "o1", "ret-1", 100))
	if a.calls.Load() != 5 || len(o.dlq) != 1 {
		t.Fatalf("calls=%d dlq=%v, want 5 attempts then DLQ", a.calls.Load(), o.parked())
	}
}

func TestConsumer_DLQOutageCommitsNothingLater(t *testing.T) {
	f := newFixture(t)
	f.paid(t, "o-after", 100)
	brokers := sharedBrokers(t)
	topic, group := newTopic()
	produce(t, brokers, topic, rec{Key: "poison", Value: []byte{0xff}}, paidRecord(t, "o-after", 100, "s1"))
	h := settlement.NewHandler(f.svc, quiet)

	down := testkit.NewRecorder()
	down.FailOn("poison")
	first := startLoop(t, brokers, topic, group, h, down)
	select {
	case err := <-first.done:
		if err == nil {
			t.Fatal("the loop must stop with an error when the dead-letter produce fails")
		}
		first.done <- err
	case <-time.After(90 * time.Second):
		t.Fatal("the loop did not stop on the dead-letter outage")
	}
	if len(down.Records()) != 0 {
		t.Fatalf("dead-lettered %v during the outage", down.Records())
	}
	if len(f.entries(t, "s1")) != 0 {
		t.Fatal("a later record was applied while an earlier one was stuck")
	}

	up := testkit.NewRecorder()
	second := startLoop(t, brokers, topic, group, h, up)
	second.waitSentinel()
	second.shutdown()
	if got := up.Records(); len(got) != 1 || string(got[0].Key) != "poison" {
		t.Fatalf("after recovery the poison record must be redelivered and parked, got %v", got)
	}
	if got := f.entries(t, "s1")[store.LedgerTypeOrderSettlement]; len(got) != 1 {
		t.Fatalf("o-after not credited once after the DLQ recovered: %+v", got)
	}
}

func TestConsumerLogsKeyFingerprintNotKey(t *testing.T) {
	a := &failingApplier{}
	brokers := sharedBrokers(t)
	topic, group := newTopic()
	poison := rec{Key: "order-123", Value: []byte("not an envelope \xff")}
	transient := paidRecord(t, "order-456", 100, "s1")
	produce(t, brokers, topic, poison, transient)
	dlq := testkit.NewRecorder()
	l := startLoop(t, brokers, topic, group, settlement.NewHandler(a, quiet), dlq)
	l.waitSentinel()
	l.shutdown()

	out := l.logs.String()
	for key, fpOf := range map[string]string{"order-123": l.fp.Of([]byte("order-123")), "order-456": l.fp.Of([]byte("order-456"))} {
		if !strings.Contains(out, "key_fp="+fpOf) {
			t.Errorf("logs must carry key_fp=%s for key %s:\n%s", fpOf, key, out)
		}
		if strings.Contains(out, key) {
			t.Errorf("logs must never contain the raw record key %s:\n%s", key, out)
		}
	}
}

func TestHandlerLogsCarryNoRawSellerOrUserID(t *testing.T) {
	const seller, other, buyer = "seller-secret-1", "seller-secret-2", "buyer-secret-1"
	paid := func(sellers ...string) rec {
		ev := &orderv1.OrderPaidEvent{OrderId: "o1", BuyerId: buyer, TotalAmount: 100, Currency: "VND"}
		for _, s := range sellers {
			ev.Items = append(ev.Items, &orderv1.OrderLineItemFact{ListingId: "l", SellerId: s, Quantity: 1, UnitPrice: 100})
		}
		return rec{Key: "o1", Value: envelope(t, settlement.OrderPaidEventType, "paid-o1", ev)}
	}
	cancelled := rec{Key: "o1-cancel", Value: envelope(t, settlement.OrderCancelledEventType, "cancel-o1",
		&orderv1.OrderCancelled{OrderId: "o1", BuyerId: buyer, SellerId: seller, PreviousStatus: orderv1.OrderStatus_ORDER_STATUS_PAID, TotalAmount: 100})}
	returned := rec{Key: "o1/r1", Value: envelope(t, settlement.ReturnRefundedEventType, "rr-r1",
		&orderv1.ReturnRefunded{ReturnId: "r1", OrderId: "o1", BuyerId: buyer, SellerId: seller, RefundAmount: 100})}
	cases := map[string]rec{
		"transient credit":  paid(seller),
		"multi seller":      paid(seller, other),
		"transient cancel":  cancelled,
		"transient return":  returned,
		"malformed payload": {Key: "bad", Value: mustMarshal(t, &eventsv1.EventEnvelope{EventId: "e", Type: settlement.OrderPaidEventType, Payload: []byte{0xff}})},
	}
	for name, r := range cases {
		t.Run(name, func(t *testing.T) {
			logs := &syncBuffer{}
			h := settlement.NewHandler(&failingApplier{}, slogTo(logs))
			err := h.Handle(context.Background(), toRecord(r))
			if err == nil {
				t.Fatal("the handler must return an error for this record")
			}
			out := logs.String() + "\n" + err.Error()
			for _, id := range []string{seller, other, buyer} {
				if strings.Contains(out, id) {
					t.Errorf("log or error output carries the raw id %q:\n%s", id, out)
				}
			}
		})
	}
}
