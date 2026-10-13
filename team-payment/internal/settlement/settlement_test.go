package settlement_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	sdkconsume "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/buidangphuc/team-payment/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	"github.com/buidangphuc/team-payment/internal/payment"
	"github.com/buidangphuc/team-payment/internal/pgtest"
	"github.com/buidangphuc/team-payment/internal/settlement"
	"github.com/buidangphuc/team-payment/internal/store"
)

// ── fakes ────────────────────────────────────────────────────────────

// ── fixtures ─────────────────────────────────────────────────────────

type fixture struct {
	svc      *payment.Service
	payments store.PaymentRepository
	ledger   store.LedgerRepository
}

func newFixture() fixture {
	p := store.NewInMemoryPaymentRepository()
	l := store.NewInMemoryLedgerRepository()
	svc := payment.NewService(p, store.NewInMemoryWalletRepository(), nil, quiet,
		payment.WithLedgerRepo(l), payment.WithSettlementLedger(store.NewInMemorySettlementLedger(p, l)))
	return fixture{svc: svc, payments: p, ledger: l}
}

// newPGFixture is the same service over Postgres stores (fresh migrated schema).
func newPGFixture(t *testing.T) fixture {
	pool := pgtest.Pool(t)
	p := store.NewPostgresPaymentRepository(pool)
	l := store.NewPostgresLedgerRepository(pool)
	svc := payment.NewService(p, store.NewPostgresWalletRepository(pool), nil, quiet,
		payment.WithLedgerRepo(l), payment.WithSettlementLedger(store.NewPostgresSettlementLedger(pool)))
	return fixture{svc: svc, payments: p, ledger: l}
}

// eachBackend runs fn in memory and, with TEST_DATABASE_URL, on Postgres.
func eachBackend(t *testing.T, fn func(t *testing.T, f fixture)) {
	t.Run("memory", func(t *testing.T) { fn(t, newFixture()) })
	t.Run("postgres", func(t *testing.T) { fn(t, newPGFixture(t)) })
}

func (f fixture) paid(t *testing.T, orderID string, amount int64) store.PaymentTransaction {
	t.Helper()
	ctx := context.Background()
	tx, err := f.payments.CreateTransaction(ctx, store.PaymentTransaction{OrderID: orderID, BuyerID: "buyer", Amount: amount})
	if err != nil {
		t.Fatal(err)
	}
	tx, err = f.payments.UpdateTransactionStatus(ctx, tx.ID, store.PaymentStatusPaid, "MOCK")
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func (f fixture) entries(t *testing.T, seller string) map[string][]store.LedgerEntry {
	t.Helper()
	all, _, err := f.ledger.ListEntries(context.Background(), seller, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]store.LedgerEntry{}
	for _, e := range all {
		out[e.Type] = append(out[e.Type], e)
	}
	return out
}

func envelope(t *testing.T, typ, eventID string, payload proto.Message) []byte {
	t.Helper()
	raw, err := proto.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	v, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: eventID, Type: typ, Payload: raw, OccurredAt: timestamppb.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func paidRecord(t *testing.T, orderID string, total int64, sellers ...string) record {
	t.Helper()
	ev := &orderv1.OrderPaidEvent{OrderId: orderID, BuyerId: "buyer", TotalAmount: total, Currency: "VND", PaidAt: timestamppb.Now()}
	for _, s := range sellers {
		ev.Items = append(ev.Items, &orderv1.OrderLineItemFact{ListingId: "l", SellerId: s, Quantity: 1, UnitPrice: total})
	}
	return record{Key: orderID, Value: envelope(t, settlement.OrderPaidEventType, "paid-"+orderID, ev)}
}

func shippedRecord(t *testing.T, orderID string) record {
	t.Helper()
	return record{Key: orderID + "-shipped", Value: envelope(t, "platform.order.v1.OrderShipped", "ship-"+orderID,
		&orderv1.OrderShipped{OrderId: orderID})}
}

// ── tests ────────────────────────────────────────────────────────────

func TestConsumer_CreditsOnceAndRedeliveryIsNoop(t *testing.T) {
	f := newFixture()
	tx := f.paid(t, "o1", 500000)
	reader := &fakeReader{records: []record{paidRecord(t, "o1", 500000, "s1", "s1"), paidRecord(t, "o1", 500000, "s1", "s1")}}
	dlq := &fakeDLQ{}
	run(t, f.svc, reader, dlq, slow, committedN(reader, 2))
	got := f.entries(t, "s1")[store.LedgerTypeOrderSettlement]
	if len(got) != 1 || got[0].Amount != 500000 || got[0].ReferenceID != tx.ID || got[0].Status != store.LedgerStatusCompleted {
		t.Fatalf("credits: %+v", got)
	}
	if len(dlq.parked()) != 0 {
		t.Fatalf("nothing may be dead-lettered: %v", dlq.parked())
	}
}

// The transaction amount wins over the event's total.
func TestConsumer_CreditsTheTransactionAmount(t *testing.T) {
	f := newFixture()
	f.paid(t, "o1", 400000)
	reader := &fakeReader{records: []record{paidRecord(t, "o1", 999, "s1")}}
	run(t, f.svc, reader, &fakeDLQ{}, slow, committedN(reader, 1))
	if got := f.entries(t, "s1")[store.LedgerTypeOrderSettlement]; len(got) != 1 || got[0].Amount != 400000 {
		t.Fatalf("credits: %+v", got)
	}
}

func TestConsumer_IgnoresOrderShipped(t *testing.T) {
	f := newFixture()
	f.paid(t, "o1", 100)
	reader := &fakeReader{records: []record{shippedRecord(t, "o1")}}
	dlq := &fakeDLQ{}
	run(t, f.svc, reader, dlq, slow, committedN(reader, 1))
	if len(f.entries(t, "s1")) != 0 || len(dlq.parked()) != 0 {
		t.Fatalf("OrderShipped must be ignored: ledger %+v dlq %v", f.entries(t, "s1"), dlq.parked())
	}
}

func TestConsumer_PoisonRecordsGoToDLQAfterOneAttempt(t *testing.T) {
	f := newFixture()
	f.paid(t, "o-ok", 100)
	f.paid(t, "o-mixed", 100)
	noEventID := record{Key: "no-event-id", Value: envelope(t, settlement.OrderPaidEventType, "", &orderv1.OrderPaidEvent{OrderId: "o-ok"})}
	badPayload := record{Key: "bad-payload", Value: mustMarshal(t, &eventsv1.EventEnvelope{EventId: "e", Type: settlement.OrderPaidEventType, Payload: []byte{0xff, 0xff}})}
	reader := &fakeReader{records: []record{
		{Key: "malformed", Value: []byte("not an envelope \xff\xfe")},
		noEventID,
		badPayload,
		paidRecord(t, "o-no-tx", 100, "s1"),
		paidRecord(t, "o-mixed", 100, "s1", "s2"),
		paidRecord(t, "o-no-seller", 100),
		paidRecord(t, "o-ok", 100, "s1"),
	}}
	dlq := &fakeDLQ{}
	run(t, f.svc, reader, dlq, slow, committedN(reader, 7))
	want := []string{"malformed", "no-event-id", "bad-payload", "o-no-tx", "o-mixed", "o-no-seller"}
	if got := dlq.parked(); len(got) != len(want) || dlq.topic != settlement.DefaultDLQTopic {
		t.Fatalf("dlq %v on %q, want %v on %q", got, dlq.topic, want, settlement.DefaultDLQTopic)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("dlq %v, want %v", got, want)
			}
		}
	}
	if got := f.entries(t, "s1")[store.LedgerTypeOrderSettlement]; len(got) != 1 || got[0].Amount != 100 {
		t.Fatalf("the paid order after the poison records must be credited once: %+v", got)
	}
	if len(f.entries(t, "s2")) != 0 {
		t.Fatal("a mixed-seller order credited a seller")
	}
}

func mustMarshal(t *testing.T, m proto.Message) []byte {
	t.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
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
	reader := &fakeReader{records: []record{paidRecord(t, "o1", 100, "s1")}}
	dlq := &fakeDLQ{}
	run(t, a, reader, dlq, sdkconsume.Options{MaxAttempts: 5, BaseBackoff: time.Millisecond}, committedN(reader, 1))
	if a.calls.Load() != 5 || len(dlq.parked()) != 1 {
		t.Fatalf("calls=%d dlq=%v, want 5 attempts then DLQ", a.calls.Load(), dlq.parked())
	}
}
