package events_test

import (
	"context"
	"errors"
	"io/fs"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/store"
	"github.com/buidangphuc/team-order/migrations"
)

const (
	paymentConsumerName = "team-order.payment"
	paymentTopic        = "payment.events"
	paymentDLQTopic     = "payment.events.dlq"
)

// orderDB starts Postgres with every migration applied.
func orderDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, testkit.Postgres(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		sqlBytes, err := fs.ReadFile(migrations.Files, n)
		if err != nil {
			t.Fatalf("read %s: %v", n, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("apply %s: %v", n, err)
		}
	}
	return pool
}

func pendingOrder(t *testing.T, orders *store.PostgresOrderRepository, voucher string) store.Order {
	t.Helper()
	o, err := orders.CreateOrder(context.Background(), store.Order{
		BuyerID: "buyer", SellerID: "seller", TotalAmount: 10, VoucherCode: voucher,
		Items: []store.OrderItem{{ListingID: "l1", Quantity: 1, UnitPrice: 10}},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	return o
}

func settledRecord(t *testing.T, eventID, orderID string) consume.Record {
	t.Helper()
	payload, err := proto.Marshal(&paymentv1.PaymentSettled{
		PaymentId: "pay-" + orderID, OrderId: orderID, Status: paymentv1.PaymentStatus_PAYMENT_STATUS_PAID,
	})
	if err != nil {
		t.Fatal(err)
	}
	value, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId: eventID, Type: "platform.payment.v1.PaymentSettled", Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	return consume.Record{Topic: paymentTopic, Key: []byte(orderID), Value: value}
}

func count(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func processedRows(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	return count(t, pool, `SELECT count(*) FROM processed_events WHERE consumer = $1 AND event_id = $2`, paymentConsumerName, eventID)
}

func outboxRows(t *testing.T, pool *pgxpool.Pool, orderID string) int {
	t.Helper()
	return count(t, pool, `SELECT count(*) FROM order_outbox_events WHERE aggregate_id = $1`, orderID)
}

func orderStatus(t *testing.T, orders *store.PostgresOrderRepository, id string) store.OrderStatus {
	t.Helper()
	o, err := orders.GetOrder(context.Background(), id)
	if err != nil {
		t.Fatalf("get order: %v", err)
	}
	return o.Status
}

func TestPaymentSettledAppliedOnceOnRedelivery(t *testing.T) {
	ctx := context.Background()
	pool := orderDB(t)
	orders := store.NewPostgresOrderRepository(pool, store.WithPaidOutbox(events.BuildPaidOutboxRow))
	c := events.NewPaymentConsumer(store.NewPostgresTransactor(pool, orders), nil)
	o := pendingOrder(t, orders, "")
	rec := settledRecord(t, "evt-1", o.ID)

	if err := c.Handle(ctx, rec); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	paid, err := orders.GetOrder(ctx, o.ID)
	if err != nil || paid.Status != store.OrderStatusPaid || paid.PaidAt == nil {
		t.Fatalf("after the first delivery: %+v, %v", paid, err)
	}

	if err := c.Handle(ctx, rec); err != nil {
		t.Fatalf("a redelivery must be acknowledged, got %v", err)
	}
	again, err := orders.GetOrder(ctx, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !again.UpdatedAt.Equal(paid.UpdatedAt) || !again.PaidAt.Equal(*paid.PaidAt) {
		t.Fatalf("the redelivery changed the order: paid at %v/%v, updated at %v/%v", paid.PaidAt, again.PaidAt, paid.UpdatedAt, again.UpdatedAt)
	}
	if n := outboxRows(t, pool, o.ID); n != 1 {
		t.Fatalf("outbox rows for the order = %d, want exactly 1 PAID fact", n)
	}
	if n := processedRows(t, pool, "evt-1"); n != 1 {
		t.Fatalf("processed_events rows = %d, want 1", n)
	}
}

func TestRolledBackTransitionIsNotDeduped(t *testing.T) {
	ctx := context.Background()
	pool := orderDB(t)
	var builds atomic.Int32
	orders := store.NewPostgresOrderRepository(pool, store.WithPaidOutbox(func(o store.Order) (store.OutboxRow, error) {
		if builds.Add(1) == 1 {
			return store.OutboxRow{}, errors.New("outbox row builder down")
		}
		return events.BuildPaidOutboxRow(o)
	}))
	c := events.NewPaymentConsumer(store.NewPostgresTransactor(pool, orders), nil)
	o := pendingOrder(t, orders, "")
	rec := settledRecord(t, "evt-rb", o.ID)

	if err := c.Handle(ctx, rec); err == nil {
		t.Fatal("a failed PAID transition must be reported so the event is retried")
	}
	if n := processedRows(t, pool, "evt-rb"); n != 0 {
		t.Fatalf("the rolled-back event was recorded as processed (%d rows)", n)
	}
	if st := orderStatus(t, orders, o.ID); st != store.OrderStatusPending {
		t.Fatalf("status after the rollback = %v, want PENDING", st)
	}

	if err := c.Handle(ctx, rec); err != nil {
		t.Fatalf("the retry: %v", err)
	}
	if st := orderStatus(t, orders, o.ID); st != store.OrderStatusPaid {
		t.Fatalf("status after the retry = %v, want PAID", st)
	}
	if n := processedRows(t, pool, "evt-rb"); n != 1 {
		t.Fatalf("processed_events rows after the retry = %d, want 1", n)
	}
}

// failingVoucher fails its first commit, then succeeds.
type failingVoucher struct {
	attempts  atomic.Int32
	committed atomic.Int32
}

func (f *failingVoucher) CommitReservation(context.Context, *promotionv1.CommitReservationRequest, ...grpc.CallOption) (*promotionv1.CommitReservationResponse, error) {
	if f.attempts.Add(1) == 1 {
		return nil, errors.New("promotion unavailable")
	}
	f.committed.Add(1)
	return &promotionv1.CommitReservationResponse{Committed: true}, nil
}

// The voucher commit is part of the delivery's transaction: when it fails, the
// whole delivery rolls back and the redelivery applies PAID and commits the
// voucher once.
func TestVoucherCommitFailureRollsBackAndRedeliveryCommitsOnce(t *testing.T) {
	ctx := context.Background()
	pool := orderDB(t)
	orders := store.NewPostgresOrderRepository(pool, store.WithPaidOutbox(events.BuildPaidOutboxRow))
	promo := &failingVoucher{}
	c := events.NewPaymentConsumer(store.NewPostgresTransactor(pool, orders), nil, events.WithVoucherCommitter(promo))
	o := pendingOrder(t, orders, "SAVE")
	rec := settledRecord(t, "evt-v", o.ID)

	if err := c.Handle(ctx, rec); err == nil {
		t.Fatal("a voucher commit error must be returned so the event is redelivered")
	}
	if st := orderStatus(t, orders, o.ID); st != store.OrderStatusPending {
		t.Fatalf("status after the failed delivery = %v, want PENDING (rolled back)", st)
	}
	if n := processedRows(t, pool, "evt-v"); n != 0 {
		t.Fatalf("processed_events rows after the failed delivery = %d, want 0", n)
	}

	if err := c.Handle(ctx, rec); err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	if err := c.Handle(ctx, rec); err != nil {
		t.Fatalf("third delivery: %v", err)
	}
	if got := promo.committed.Load(); got != 1 {
		t.Fatalf("voucher committed %d times, want exactly once", got)
	}
	if st := orderStatus(t, orders, o.ID); st != store.OrderStatusPaid {
		t.Fatalf("final status = %v, want PAID", st)
	}
}

// failingDLQ is a dead-letter producer whose broker is down.
type failingDLQ struct{}

func (failingDLQ) PublishRecord(context.Context, *kgo.Record) error {
	return errors.New("dlq broker unavailable")
}

func TestPaymentEventNotLostWhenDLQFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	brokers := testkit.Redpanda(t)
	cfg := sdkconfig.Kafka{Brokers: brokers}

	pool := orderDB(t)
	orders := store.NewPostgresOrderRepository(pool, store.WithPaidOutbox(events.BuildPaidOutboxRow))
	handler := events.NewPaymentConsumer(store.NewPostgresTransactor(pool, orders), nil)

	producer, err := kafkax.NewProducer(cfg, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()

	// A settlement for an order that does not exist can never succeed: a permanent
	// failure that must reach the dead-letter topic.
	rec := settledRecord(t, "evt-lost", "ord-missing")
	if err := producer.Publish(ctx, paymentTopic, []byte("ord-missing"), rec.Value); err != nil {
		t.Fatalf("seed payment.events: %v", err)
	}

	const group = "team-order.payment.dlq-test"
	opts := consume.Options{DLQTopic: paymentDLQTopic, MaxAttempts: 1}

	// First run: the handler fails permanently and the DLQ produce fails too.
	first, err := kafkax.NewConsumerClient(cfg, group, []string{paymentTopic})
	if err != nil {
		t.Fatal(err)
	}
	runCtx, stop := context.WithTimeout(ctx, 90*time.Second)
	defer stop()
	runErr := consume.NewConsumer(first, handler.Handle, failingDLQ{}, opts).Run(runCtx)
	first.Close()
	if runErr == nil || runCtx.Err() != nil {
		t.Fatalf("the consumer must stop with an error when the DLQ produce fails, got %v (ctx %v)", runErr, runCtx.Err())
	}

	// Restart in the same group with a working DLQ: the event is delivered again
	// and dead-lettered, so it was not lost.
	second, err := kafkax.NewConsumerClient(cfg, group, []string{paymentTopic})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	dlq, err := kafkax.NewProducer(cfg, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatal(err)
	}
	defer dlq.Close()
	restartCtx, stopRestart := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- consume.NewConsumer(second, handler.Handle, dlq, opts).Run(restartCtx) }()
	defer func() { stopRestart(); <-done }()

	reader, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.ConsumeTopics(paymentDLQTopic), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	pollCtx, stopPoll := context.WithTimeout(ctx, 90*time.Second)
	defer stopPoll()
	for {
		fetches := reader.PollFetches(pollCtx)
		if pollCtx.Err() != nil {
			t.Fatal("the payment event never reached the dead-letter topic after the restart")
		}
		var got *kgo.Record
		fetches.EachRecord(func(r *kgo.Record) { got = r })
		if got != nil {
			if string(got.Value) != string(rec.Value) {
				t.Fatal("the dead-lettered record differs from the consumed one")
			}
			return
		}
	}
}
