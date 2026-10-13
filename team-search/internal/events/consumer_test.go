package events_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	eventsv1 "github.com/buidangphuc/team-search/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-search/generated/platform/listing/v1"
	"github.com/buidangphuc/team-search/internal/events"
	"github.com/buidangphuc/team-search/internal/index"
)

const waitFor = 60 * time.Second

// fakeDLQ records the records parked to the dead-letter topic and can be made to
// fail its publish to simulate an unreachable DLQ.
type fakeDLQ struct {
	mu         sync.Mutex
	records    [][2][]byte
	publishErr error
	parked     chan struct{}
}

func newFakeDLQ(publishErr error) *fakeDLQ {
	return &fakeDLQ{publishErr: publishErr, parked: make(chan struct{}, 8)}
}

func (f *fakeDLQ) PublishRecord(_ context.Context, r *kgo.Record) error {
	if f.publishErr != nil {
		return f.publishErr
	}
	f.mu.Lock()
	f.records = append(f.records, [2][]byte{r.Key, r.Value})
	f.mu.Unlock()
	f.parked <- struct{}{}
	return nil
}

func (f *fakeDLQ) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.records)
}

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func fastOpts(topic string) consume.Options {
	return consume.Options{
		DLQTopic: topic + ".dlq", MaxAttempts: 3,
		BaseBackoff: time.Microsecond, MaxBackoff: time.Millisecond, Logger: discardLogger(),
	}
}

func consumerClient(t *testing.T, brokers, group, topic string) *kgo.Client {
	t.Helper()
	c, err := kafkax.NewConsumerClient(config.Kafka{Brokers: brokers}, group, []string{topic},
		kgo.AllowAutoTopicCreation(), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatalf("consumer client: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

func produce(t *testing.T, brokers, topic string, key, value []byte) {
	t.Helper()
	p, err := kafkax.NewProducer(config.Kafka{Brokers: brokers}, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatalf("producer: %v", err)
	}
	defer p.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := p.Publish(ctx, topic, key, value); err != nil {
		t.Fatalf("produce: %v", err)
	}
}

// newConsumer is events.NewConsumerWithDLQ failing the test on a build error.
func newConsumer(t *testing.T, client *kgo.Client, dlq consume.DLQProducer, h events.Handler, o consume.Options) *events.Consumer {
	t.Helper()
	c, err := events.NewConsumerWithDLQ(client, dlq, h, o)
	if err != nil {
		t.Fatalf("new consumer: %v", err)
	}
	return c
}

// run starts the consumer and returns a stop func that cancels it and returns
// Run's result, and the channel Run's result arrives on by itself.
func run(c *events.Consumer) (stop func() error, done <-chan error) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan error, 1)
	go func() { ch <- c.Run(ctx) }()
	return func() error { cancel(); return <-ch }, ch
}

// AD1 / SA-C1: a permanently failing record is retried MaxAttempts times, then
// parked to the DLQ with its original key and value, and only then may the
// offset advance.
func TestProcessRecord_ParksToDLQAfterMaxAttempts(t *testing.T) {
	t.Parallel()
	brokers := testkit.Redpanda(t)
	const topic = "listing.events"
	dlq := newFakeDLQ(nil)
	var mu sync.Mutex
	var calls int
	h := func(_ context.Context, _, _ []byte) error {
		mu.Lock()
		calls++
		mu.Unlock()
		return errors.New("sink down")
	}
	c := newConsumer(t, consumerClient(t, brokers, "g-park", topic), dlq, h, fastOpts(topic))
	stop, done := run(c)
	produce(t, brokers, topic, []byte("k"), []byte("v"))

	select {
	case <-dlq.parked:
	case err := <-done:
		t.Fatalf("Run returned %v before the record was parked", err)
	case <-time.After(waitFor):
		t.Fatal("record was never parked to the dlq")
	}
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 3 {
		t.Fatalf("expected 3 handler attempts, got %d", calls)
	}
	if dlq.count() != 1 {
		t.Fatalf("expected 1 record parked to dlq, got %d", dlq.count())
	}
	if got := string(dlq.records[0][0]); got != "k" {
		t.Errorf("dlq key = %q, want %q", got, "k")
	}
	if got := string(dlq.records[0][1]); got != "v" {
		t.Errorf("dlq value = %q, want %q", got, "v")
	}
}

// AD1 / SA-C1: when the record can neither be handled nor parked (DLQ down), Run
// returns an error so the offset does not advance past the failed record.
func TestProcessRecord_DLQFailureBlocksAdvance(t *testing.T) {
	t.Parallel()
	brokers := testkit.Redpanda(t)
	const topic = "listing.events"
	dlq := newFakeDLQ(errors.New("dlq unreachable"))
	h := func(_ context.Context, _, _ []byte) error { return errors.New("sink down") }
	c := newConsumer(t, consumerClient(t, brokers, "g-block", topic), dlq, h, fastOpts(topic))
	_, done := run(c)
	produce(t, brokers, topic, []byte("k"), []byte("v"))

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error so the offset does not advance past the failed record")
		}
	case <-time.After(waitFor):
		t.Fatal("Run kept going although the record could be neither handled nor parked")
	}
	if dlq.count() != 0 {
		t.Fatalf("expected no record recorded on dlq failure, got %d", dlq.count())
	}
}

// Spec: team-search consumer does not commit when the DLQ produce fails; the
// record is redelivered to the next consumer of the same group after restart.
func TestConsumerDoesNotCommitOnDLQFailure(t *testing.T) {
	t.Parallel()
	brokers := testkit.Redpanda(t)
	const topic = "listing.events"
	const group = "g-redeliver"

	failing := func(_ context.Context, _, _ []byte) error { return errors.New("sink down") }
	first := consumerClient(t, brokers, group, topic)
	c := newConsumer(t, first, newFakeDLQ(errors.New("dlq unreachable")), failing, fastOpts(topic))
	_, done := run(c)
	produce(t, brokers, topic, []byte("k"), []byte("v"))
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run must stop with an error when the DLQ produce fails")
		}
	case <-time.After(waitFor):
		t.Fatal("Run did not stop after the DLQ produce failed")
	}
	first.Close() // leave the group, as a restart would

	got := make(chan string, 1)
	ok := func(_ context.Context, _, value []byte) error { got <- string(value); return nil }
	second := newConsumer(t, consumerClient(t, brokers, group, topic), newFakeDLQ(nil), ok, fastOpts(topic))
	stop, done2 := run(second)
	defer func() { _ = stop() }()
	select {
	case v := <-got:
		if v != "v" {
			t.Fatalf("redelivered %q, want v", v)
		}
	case err := <-done2:
		t.Fatalf("second consumer stopped: %v", err)
	case <-time.After(waitFor):
		t.Fatal("the record was not redelivered after the restart: it was committed past")
	}
}

// A record the handler accepts is handled once, with no retry and no DLQ.
func TestProcessRecord_SuccessNoDLQ(t *testing.T) {
	t.Parallel()
	brokers := testkit.Redpanda(t)
	const topic = "listing.events"
	dlq := newFakeDLQ(nil)
	handled := make(chan struct{}, 4)
	h := func(_ context.Context, _, _ []byte) error { handled <- struct{}{}; return nil }
	c := newConsumer(t, consumerClient(t, brokers, "g-ok", topic), dlq, h, fastOpts(topic))
	stop, done := run(c)
	produce(t, brokers, topic, []byte("k"), []byte("v"))
	select {
	case <-handled:
	case err := <-done:
		t.Fatalf("Run returned %v", err)
	case <-time.After(waitFor):
		t.Fatal("record was never handled")
	}
	time.Sleep(500 * time.Millisecond) // a retry would show up here
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(handled) != 0 {
		t.Fatalf("expected 1 handler attempt, got %d", 1+len(handled))
	}
	if dlq.count() != 0 {
		t.Fatalf("expected no dlq records on success, got %d", dlq.count())
	}
}

// A retry that eventually succeeds does not reach the DLQ.
func TestProcessRecord_RecoversBeforeDLQ(t *testing.T) {
	t.Parallel()
	brokers := testkit.Redpanda(t)
	const topic = "listing.events"
	dlq := newFakeDLQ(nil)
	var mu sync.Mutex
	var calls int
	recovered := make(chan struct{}, 1)
	h := func(_ context.Context, _, _ []byte) error {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls < 2 {
			return errors.New("transient")
		}
		recovered <- struct{}{}
		return nil
	}
	c := newConsumer(t, consumerClient(t, brokers, "g-recover", topic), dlq, h, fastOpts(topic))
	stop, done := run(c)
	produce(t, brokers, topic, []byte("k"), []byte("v"))
	select {
	case <-recovered:
	case err := <-done:
		t.Fatalf("Run returned %v", err)
	case <-time.After(waitFor):
		t.Fatal("record was never handled")
	}
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Fatalf("expected 2 handler attempts, got %d", calls)
	}
	if dlq.count() != 0 {
		t.Fatalf("expected no dlq records when handler recovers, got %d", dlq.count())
	}
}

// stockOnlyIndex counts UpdateStock calls; any other method panics via the nil
// embedded interface, which would fail the test.
type stockOnlyIndex struct {
	index.Index
	mu    sync.Mutex
	calls int
}

func (s *stockOnlyIndex) UpdateStock(context.Context, string, int32, int64) error {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return nil
}

// D2 + AD1: a malformed ListingStockChanged is retried, then parked on the DLQ
// with its original key and bytes, and never applied.
func TestProcessRecord_MalformedStockEventReachesDLQ(t *testing.T) {
	t.Parallel()
	brokers := testkit.Redpanda(t)
	const topic = "listing.events"
	payload, _ := proto.Marshal(&listingv1.ListingStockChanged{ListingId: "l1", Stock: -1})
	value, _ := proto.Marshal(&eventsv1.EventEnvelope{
		Type: "platform.listing.v1.ListingStockChanged", OccurredAt: timestamppb.New(time.Unix(1700000000, 0)), Payload: payload,
	})
	idx := &stockOnlyIndex{}
	dlq := newFakeDLQ(nil)
	c := newConsumer(t, consumerClient(t, brokers, "g-stock", topic), dlq, events.ListingEventHandler(idx), fastOpts(topic))
	stop, done := run(c)
	produce(t, brokers, topic, []byte("l1"), value)
	select {
	case <-dlq.parked:
	case err := <-done:
		t.Fatalf("Run returned %v", err)
	case <-time.After(waitFor):
		t.Fatal("the malformed record was never parked")
	}
	if err := stop(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if dlq.count() != 1 || string(dlq.records[0][0]) != "l1" || string(dlq.records[0][1]) != string(value) {
		t.Fatalf("want the record parked unchanged, got %d records", dlq.count())
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.calls != 0 {
		t.Errorf("malformed stock event was applied %d times", idx.calls)
	}
}

// The SDK now reports a bad consumer instead of panicking later: a nil client
// or an empty dead-letter topic is an error from the constructor.
func TestNewConsumerWithDLQ_RejectsInvalidInput(t *testing.T) {
	t.Parallel()
	h := func(context.Context, []byte, []byte) error { return nil }
	if _, err := events.NewConsumerWithDLQ(nil, newFakeDLQ(nil), h, fastOpts("listing.events")); err == nil {
		t.Error("a nil client must be an error")
	}
	if _, err := events.NewConsumerWithDLQ(nil, newFakeDLQ(nil), h, consume.Options{}); err == nil {
		t.Error("an empty DLQTopic must be an error")
	}
}
