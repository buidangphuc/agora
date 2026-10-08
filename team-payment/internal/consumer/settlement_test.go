package consumer_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/buidangphuc/team-payment/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	"github.com/buidangphuc/team-payment/internal/consumer"
	"github.com/buidangphuc/team-payment/internal/pgtest"
	"github.com/buidangphuc/team-payment/internal/repository"
	"github.com/buidangphuc/team-payment/internal/service"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// ── fakes ────────────────────────────────────────────────────────────

type fakeReader struct {
	mu        sync.Mutex
	records   []consumer.Record
	next      int
	committed []string
}

func (r *fakeReader) Fetch(ctx context.Context) (consumer.Record, error) {
	for {
		r.mu.Lock()
		if r.next < len(r.records) {
			rec := r.records[r.next]
			r.next++
			r.mu.Unlock()
			return rec, nil
		}
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return consumer.Record{}, ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func (r *fakeReader) Commit(_ context.Context, rec consumer.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.committed = append(r.committed, rec.Key)
	return nil
}

func (r *fakeReader) commits() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.committed...)
}

type fakeDLQ struct {
	mu    sync.Mutex
	fail  bool
	topic string
	keys  []string
}

func (d *fakeDLQ) Produce(_ context.Context, topic string, rec consumer.Record) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail {
		return errors.New("broker down")
	}
	d.topic = topic
	d.keys = append(d.keys, rec.Key)
	return nil
}

func (d *fakeDLQ) parked() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.keys...)
}

func (d *fakeDLQ) setFail(f bool) {
	d.mu.Lock()
	d.fail = f
	d.mu.Unlock()
}

// ── fixtures ─────────────────────────────────────────────────────────

type fixture struct {
	svc      *service.PaymentService
	payments repository.PaymentRepository
	ledger   repository.LedgerRepository
}

func newFixture() fixture {
	p := repository.NewInMemoryPaymentRepository()
	l := repository.NewInMemoryLedgerRepository()
	svc := service.NewPaymentService(p, repository.NewInMemoryWalletRepository(), nil, quiet,
		service.WithLedgerRepo(l), service.WithSettlementLedger(repository.NewInMemorySettlementLedger(p, l)))
	return fixture{svc: svc, payments: p, ledger: l}
}

// newPGFixture is the same service over Postgres stores (fresh migrated schema).
func newPGFixture(t *testing.T) fixture {
	pool := pgtest.Pool(t)
	p := repository.NewPostgresPaymentRepository(pool)
	l := repository.NewPostgresLedgerRepository(pool)
	svc := service.NewPaymentService(p, repository.NewPostgresWalletRepository(pool), nil, quiet,
		service.WithLedgerRepo(l), service.WithSettlementLedger(repository.NewPostgresSettlementLedger(pool)))
	return fixture{svc: svc, payments: p, ledger: l}
}

// eachBackend runs fn in memory and, with TEST_DATABASE_URL, on Postgres.
func eachBackend(t *testing.T, fn func(t *testing.T, f fixture)) {
	t.Run("memory", func(t *testing.T) { fn(t, newFixture()) })
	t.Run("postgres", func(t *testing.T) { fn(t, newPGFixture(t)) })
}

func (f fixture) paid(t *testing.T, orderID string, amount int64) repository.PaymentTransaction {
	t.Helper()
	ctx := context.Background()
	tx, err := f.payments.CreateTransaction(ctx, repository.PaymentTransaction{OrderID: orderID, BuyerID: "buyer", Amount: amount})
	if err != nil {
		t.Fatal(err)
	}
	tx, err = f.payments.UpdateTransactionStatus(ctx, tx.ID, repository.PaymentStatusPaid, "MOCK")
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func (f fixture) entries(t *testing.T, seller string) map[string][]repository.LedgerEntry {
	t.Helper()
	all, _, err := f.ledger.ListEntries(context.Background(), seller, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]repository.LedgerEntry{}
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

func paidRecord(t *testing.T, orderID string, total int64, sellers ...string) consumer.Record {
	t.Helper()
	ev := &orderv1.OrderPaidEvent{OrderId: orderID, BuyerId: "buyer", TotalAmount: total, Currency: "VND", PaidAt: timestamppb.Now()}
	for _, s := range sellers {
		ev.Items = append(ev.Items, &orderv1.OrderLineItemFact{ListingId: "l", SellerId: s, Quantity: 1, UnitPrice: total})
	}
	return consumer.Record{Key: orderID, Value: envelope(t, consumer.OrderPaidEventType, "paid-"+orderID, ev)}
}

func shippedRecord(t *testing.T, orderID string) consumer.Record {
	t.Helper()
	return consumer.Record{Key: orderID + "-shipped", Value: envelope(t, "platform.order.v1.OrderShipped", "ship-"+orderID,
		&orderv1.OrderShipped{OrderId: orderID})}
}

// run drives the loop until cond holds (or fails after 3s), then stops it.
func run(t *testing.T, apply consumer.Applier, reader *fakeReader, dlq *fakeDLQ, cfg consumer.RunConfig, cond func() bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- consumer.NewSettlementConsumer(apply, quiet).Run(ctx, reader, dlq, cfg) }()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatalf("condition not met; committed=%v dlq=%v", reader.commits(), dlq.parked())
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v", err)
	}
}

func committedN(r *fakeReader, n int) func() bool {
	return func() bool { return len(r.commits()) >= n }
}

// slow makes any retry visible: a retried record would block the test for an hour.
var slow = consumer.RunConfig{BaseBackoff: time.Hour, FetchPause: 10 * time.Millisecond}

// ── tests ────────────────────────────────────────────────────────────

func TestConsumer_CreditsOnceAndRedeliveryIsNoop(t *testing.T) {
	f := newFixture()
	tx := f.paid(t, "o1", 500000)
	reader := &fakeReader{records: []consumer.Record{paidRecord(t, "o1", 500000, "s1", "s1"), paidRecord(t, "o1", 500000, "s1", "s1")}}
	dlq := &fakeDLQ{}
	run(t, f.svc, reader, dlq, slow, committedN(reader, 2))
	got := f.entries(t, "s1")[repository.LedgerTypeOrderSettlement]
	if len(got) != 1 || got[0].Amount != 500000 || got[0].ReferenceID != tx.ID || got[0].Status != repository.LedgerStatusCompleted {
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
	reader := &fakeReader{records: []consumer.Record{paidRecord(t, "o1", 999, "s1")}}
	run(t, f.svc, reader, &fakeDLQ{}, slow, committedN(reader, 1))
	if got := f.entries(t, "s1")[repository.LedgerTypeOrderSettlement]; len(got) != 1 || got[0].Amount != 400000 {
		t.Fatalf("credits: %+v", got)
	}
}

func TestConsumer_IgnoresOrderShipped(t *testing.T) {
	f := newFixture()
	f.paid(t, "o1", 100)
	reader := &fakeReader{records: []consumer.Record{shippedRecord(t, "o1")}}
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
	noEventID := consumer.Record{Key: "no-event-id", Value: envelope(t, consumer.OrderPaidEventType, "", &orderv1.OrderPaidEvent{OrderId: "o-ok"})}
	badPayload := consumer.Record{Key: "bad-payload", Value: mustMarshal(t, &eventsv1.EventEnvelope{EventId: "e", Type: consumer.OrderPaidEventType, Payload: []byte{0xff, 0xff}})}
	reader := &fakeReader{records: []consumer.Record{
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
	if got := dlq.parked(); len(got) != len(want) || dlq.topic != consumer.DefaultDLQTopic {
		t.Fatalf("dlq %v on %q, want %v on %q", got, dlq.topic, want, consumer.DefaultDLQTopic)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("dlq %v, want %v", got, want)
			}
		}
	}
	if got := f.entries(t, "s1")[repository.LedgerTypeOrderSettlement]; len(got) != 1 || got[0].Amount != 100 {
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

func TestConsumer_TransientErrorRetriedThenDLQ(t *testing.T) {
	a := &failingApplier{}
	reader := &fakeReader{records: []consumer.Record{paidRecord(t, "o1", 100, "s1")}}
	dlq := &fakeDLQ{}
	run(t, a, reader, dlq, consumer.RunConfig{MaxAttempts: 5, BaseBackoff: time.Millisecond}, committedN(reader, 1))
	if a.calls.Load() != 5 || len(dlq.parked()) != 1 {
		t.Fatalf("calls=%d dlq=%v, want 5 attempts then DLQ", a.calls.Load(), dlq.parked())
	}
}

func TestConsumer_DLQOutageCommitsNothingLater(t *testing.T) {
	f := newFixture()
	f.paid(t, "o-after", 100)
	reader := &fakeReader{records: []consumer.Record{{Key: "malformed", Value: []byte{0xff}}, paidRecord(t, "o-after", 100, "s1")}}
	dlq := &fakeDLQ{fail: true}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- consumer.NewSettlementConsumer(f.svc, quiet).Run(ctx, reader, dlq, slow) }()
	time.Sleep(150 * time.Millisecond) // several DLQ retries
	if c := reader.commits(); len(c) != 0 {
		t.Fatalf("committed %v during the DLQ outage", c)
	}
	if len(f.entries(t, "s1")) != 0 {
		t.Fatal("a later record was applied while an earlier one was stuck")
	}
	dlq.setFail(false)
	deadline := time.Now().Add(3 * time.Second)
	for len(reader.commits()) < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	if c := reader.commits(); len(c) != 2 || c[0] != "malformed" || c[1] != "o-after" {
		t.Fatalf("commits %v, want [malformed o-after]", c)
	}
	if len(f.entries(t, "s1")[repository.LedgerTypeOrderSettlement]) != 1 {
		t.Fatal("o-after not credited once after the DLQ recovered")
	}
}
