package settlement_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	eventsv1 "github.com/buidangphuc/team-payment/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	"github.com/buidangphuc/team-payment/internal/payment"
	"github.com/buidangphuc/team-payment/internal/settlement"
	"github.com/buidangphuc/team-payment/internal/store"
	"github.com/buidangphuc/team-payment/internal/store/fake"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type keepTB struct{ testing.TB }

func (keepTB) Cleanup(func()) {}

var (
	brokersOnce sync.Once
	brokersAddr string
	seq         atomic.Int64
)

func sharedBrokers(t *testing.T) string {
	t.Helper()
	brokersOnce.Do(func() { brokersAddr = testkit.Redpanda(keepTB{t}) })
	if brokersAddr == "" {
		t.Fatal("redpanda is not available")
	}
	return brokersAddr
}

type rec struct {
	Key   string
	Value []byte
}

const sentinelKey = "zz-sentinel"

type loop struct {
	t        *testing.T
	topic    string
	group    string
	brokers  string
	logs     *syncBuffer
	fp       *kafkax.Fingerprinter
	done     chan error
	reached  chan struct{}
	stop     context.CancelFunc
	mu       sync.Mutex
	attempts map[string]int
}

func newTopic() (topic, group string) {
	n := seq.Add(1)
	return fmt.Sprintf("order.events.t%d", n), fmt.Sprintf("team-payment.settlement.t%d", n)
}

func produce(t *testing.T, brokers, topic string, recs ...rec) {
	t.Helper()
	for _, r := range recs {
		testkit.Produce(t, brokers, topic, r.Key, r.Value)
	}
	testkit.Produce(t, brokers, topic, sentinelKey, envelope(t, "platform.order.v1.OrderShipped", "ship-sentinel",
		&orderv1.OrderShipped{OrderId: "sentinel"}))
}

func startLoop(t *testing.T, brokers, topic, group string, h *settlement.Handler, dlq consume.DLQProducer) *loop {
	t.Helper()
	fp, err := kafkax.NewFingerprinter("settlement-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	l := &loop{t: t, topic: topic, group: group, brokers: brokers, logs: &syncBuffer{}, fp: fp,
		done: make(chan error, 1), reached: make(chan struct{}), attempts: map[string]int{}}
	logger := slog.New(slog.NewTextHandler(l.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	client, err := kafkax.NewConsumerClient(config.Kafka{Brokers: brokers}, group, []string{topic},
		kgo.AllowAutoTopicCreation(), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatalf("consumer client: %v", err)
	}
	var once sync.Once
	wrapped := func(ctx context.Context, r consume.Record) error {
		l.mu.Lock()
		l.attempts[string(r.Key)]++
		l.mu.Unlock()
		err := h.Handle(ctx, r)
		if string(r.Key) == sentinelKey && err == nil {
			once.Do(func() { close(l.reached) })
		}
		return err
	}
	c, err := consume.NewConsumer(client, wrapped, dlq, consume.Options{
		DLQTopic:    settlement.DefaultDLQTopic,
		BaseBackoff: time.Millisecond,
		MaxBackoff:  2 * time.Millisecond,
		Logger:      logger,
		KeyFP:       fp,
	})
	if err != nil {
		client.Close()
		t.Fatalf("consumer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	l.stop = cancel
	go func() {
		l.done <- c.Run(ctx)
		client.Close()
	}()
	t.Cleanup(func() { l.shutdown() })
	return l
}

func (l *loop) shutdown() {
	l.stop()
	select {
	case err := <-l.done:
		l.done <- err
	case <-time.After(30 * time.Second):
		l.t.Error("consumer did not stop")
	}
}

func (l *loop) waitSentinel() {
	l.t.Helper()
	select {
	case <-l.reached:
	case err := <-l.done:
		l.done <- err
		l.t.Fatalf("consumer stopped before the last record: %v", err)
	case <-time.After(90 * time.Second):
		l.t.Fatal("consumer did not reach the last record")
	}
}

func (l *loop) attemptsOf(key string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.attempts[key]
}

type outcome struct {
	dlq  []testkit.Record
	loop *loop
}

func (o *outcome) parked() []string {
	var out []string
	for _, r := range o.dlq {
		out = append(out, string(r.Key))
	}
	return out
}

func feedHandler(t *testing.T, h *settlement.Handler, recs ...rec) *outcome {
	t.Helper()
	brokers := sharedBrokers(t)
	topic, group := newTopic()
	produce(t, brokers, topic, recs...)
	dlq := testkit.NewRecorder()
	l := startLoop(t, brokers, topic, group, h, dlq)
	l.waitSentinel()
	l.shutdown()
	return &outcome{dlq: dlq.Records(), loop: l}
}

func feed(t *testing.T, f fixture, recs ...rec) *outcome {
	t.Helper()
	return feedHandler(t, settlement.NewHandler(f.svc, quiet), recs...)
}

func wantNoDLQ(t *testing.T, o *outcome) {
	t.Helper()
	if got := o.parked(); len(got) != 0 {
		t.Fatalf("dlq %v", got)
	}
}

type fixture struct {
	svc      *payment.Service
	payments store.PaymentRepository
	ledger   store.LedgerRepository
}

type noOrders struct{}

func (noOrders) GetOrder(context.Context, *orderv1.GetOrderRequest, ...grpc.CallOption) (*orderv1.GetOrderResponse, error) {
	return nil, fmt.Errorf("order service is not used by the settlement handler")
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	p := fake.NewInMemoryPaymentRepository()
	l := fake.NewInMemoryLedgerRepository()
	svc, err := payment.NewService(p, fake.NewInMemoryWalletRepository(), noOrders{}, quiet,
		fake.NewInMemoryTxWriter(p), l, fake.NewInMemorySettlementLedger(p, l))
	if err != nil {
		t.Fatal(err)
	}
	return fixture{svc: svc, payments: p, ledger: l}
}

func migratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testkit.Schema(t, testkit.SharedPostgres(t))
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("find migrations: %v (n=%d)", err, len(files))
	}
	sort.Strings(files)
	for _, f := range files {
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if _, err := pool.Exec(context.Background(), string(body)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(f), err)
		}
	}
	return pool
}

func newPGFixture(t *testing.T) fixture {
	t.Helper()
	pool := migratedPool(t)
	ob, err := outbox.NewStore(pool, "payment_outbox_events")
	if err != nil {
		t.Fatalf("outbox store: %v", err)
	}
	p := store.NewPostgresPaymentRepository(pool)
	l := store.NewPostgresLedgerRepository(pool)
	svc, err := payment.NewService(p, store.NewPostgresWalletRepository(pool), noOrders{}, quiet,
		store.NewPgTxWriter(pool, ob), l, store.NewPostgresSettlementLedger(pool))
	if err != nil {
		t.Fatal(err)
	}
	return fixture{svc: svc, payments: p, ledger: l}
}

func eachBackend(t *testing.T, fn func(t *testing.T, f fixture)) {
	t.Run("memory", func(t *testing.T) { fn(t, newFixture(t)) })
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

func mustMarshal(t *testing.T, m proto.Message) []byte {
	t.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func envelope(t *testing.T, typ, eventID string, payload proto.Message) []byte {
	t.Helper()
	return mustMarshal(t, &eventsv1.EventEnvelope{EventId: eventID, Type: typ, Payload: mustMarshal(t, payload), OccurredAt: timestamppb.Now()})
}

func paidRecord(t *testing.T, orderID string, total int64, sellers ...string) rec {
	t.Helper()
	ev := &orderv1.OrderPaidEvent{OrderId: orderID, BuyerId: "buyer", TotalAmount: total, Currency: "VND", PaidAt: timestamppb.Now()}
	for _, s := range sellers {
		ev.Items = append(ev.Items, &orderv1.OrderLineItemFact{ListingId: "l", SellerId: s, Quantity: 1, UnitPrice: total})
	}
	return rec{Key: orderID, Value: envelope(t, settlement.OrderPaidEventType, "paid-"+orderID, ev)}
}

func shippedRecord(t *testing.T, orderID string) rec {
	t.Helper()
	return rec{Key: orderID + "-shipped", Value: envelope(t, "platform.order.v1.OrderShipped", "ship-"+orderID,
		&orderv1.OrderShipped{OrderId: orderID})}
}

func cancelledRecord(t *testing.T, orderID string, from orderv1.OrderStatus, total int64) rec {
	t.Helper()
	return rec{Key: orderID + "-cancel", Value: envelope(t, settlement.OrderCancelledEventType, "cancel-"+orderID,
		&orderv1.OrderCancelled{OrderId: orderID, BuyerId: "buyer", SellerId: "s1", PreviousStatus: from,
			TotalAmount: total, Currency: "VND", CancelledAt: timestamppb.Now()})}
}

func returnRecord(t *testing.T, orderID, returnID string, amount int64) rec {
	t.Helper()
	return rec{Key: orderID + "/" + returnID, Value: envelope(t, settlement.ReturnRefundedEventType, "rr-"+returnID,
		&orderv1.ReturnRefunded{ReturnId: returnID, OrderId: orderID, BuyerId: "buyer", SellerId: "s1",
			RefundAmount: amount, Currency: "VND", RefundedAt: timestamppb.Now()})}
}

func slogTo(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func toRecord(r rec) consume.Record {
	return consume.Record{Topic: settlement.DefaultTopic, Key: []byte(r.Key), Value: r.Value}
}
