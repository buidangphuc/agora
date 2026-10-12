package consume_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"
)

const waitFor = 25 * time.Second

type fakeDLQ struct {
	mu   sync.Mutex
	err  error
	recs []*kgo.Record
}

func (f *fakeDLQ) PublishRecord(_ context.Context, r *kgo.Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.recs = append(f.recs, r)
	return nil
}

func (f *fakeDLQ) records() []*kgo.Record {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*kgo.Record(nil), f.recs...)
}

func produce(t *testing.T, brokers, topic, key, value string, headers ...kgo.RecordHeader) {
	t.Helper()
	p, err := kafkax.NewProducer(config.Kafka{Brokers: brokers}, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatalf("producer: %v", err)
	}
	defer p.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := p.PublishRecord(ctx, &kgo.Record{Topic: topic, Key: []byte(key), Value: []byte(value), Headers: headers}); err != nil {
		t.Fatalf("produce: %v", err)
	}
}

func opts() consume.Options {
	return consume.Options{
		DLQTopic:    "orders.dlq",
		MaxAttempts: 3,
		BaseBackoff: time.Millisecond,
		MaxBackoff:  5 * time.Millisecond,
	}
}

func TestPermanentErrorGoesToDLQ(t *testing.T) {
	brokers := testkit.Redpanda(t)
	produce(t, brokers, "orders", "k1", "v1", kgo.RecordHeader{Key: "x-h", Value: []byte("hv")})

	client, err := kafkax.NewConsumerClient(config.Kafka{Brokers: brokers}, "g-perm", []string{"orders"})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer client.Close()

	var mu sync.Mutex
	calls := 0
	dlq := &fakeDLQ{}
	c := consume.NewConsumer(client, func(context.Context, consume.Record) error {
		mu.Lock()
		calls++
		mu.Unlock()
		return consume.Permanent(errors.New("bad payload"))
	}, dlq, opts())

	ctx, cancel := context.WithTimeout(context.Background(), waitFor)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()

	deadline := time.Now().Add(waitFor)
	for len(dlq.records()) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}

	recs := dlq.records()
	if len(recs) != 1 {
		t.Fatalf("dlq records = %d, want 1", len(recs))
	}
	r := recs[0]
	if r.Topic != "orders.dlq" || string(r.Key) != "k1" || string(r.Value) != "v1" {
		t.Fatalf("dlq record = %s %q %q", r.Topic, r.Key, r.Value)
	}
	h := map[string]string{}
	for _, x := range r.Headers {
		h[x.Key] = string(x.Value)
	}
	if h["x-h"] != "hv" || h["source-topic"] != "orders" || h["error"] == "" {
		t.Fatalf("dlq headers = %v", h)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1 (permanent errors are not retried)", calls)
	}
}

func TestTransientErrorRetriesThenParks(t *testing.T) {
	brokers := testkit.Redpanda(t)
	produce(t, brokers, "orders", "k1", "v1")

	client, err := kafkax.NewConsumerClient(config.Kafka{Brokers: brokers}, "g-retry", []string{"orders"})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer client.Close()

	var mu sync.Mutex
	calls := 0
	dlq := &fakeDLQ{}
	c := consume.NewConsumer(client, func(context.Context, consume.Record) error {
		mu.Lock()
		calls++
		mu.Unlock()
		return errors.New("flaky")
	}, dlq, opts())

	ctx, cancel := context.WithTimeout(context.Background(), waitFor)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	deadline := time.Now().Add(waitFor)
	for len(dlq.records()) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 3 || len(dlq.records()) != 1 {
		t.Fatalf("calls = %d dlq = %d, want 3 and 1", calls, len(dlq.records()))
	}
}

func TestDLQFailureDoesNotCommit(t *testing.T) {
	brokers := testkit.Redpanda(t)
	produce(t, brokers, "orders", "k1", "v1")
	kc := config.Kafka{Brokers: brokers}

	first, err := kafkax.NewConsumerClient(kc, "g-dlqfail", []string{"orders"})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	dlqErr := errors.New("dlq down")
	c := consume.NewConsumer(first, func(context.Context, consume.Record) error {
		return consume.Permanent(errors.New("bad"))
	}, &fakeDLQ{err: dlqErr}, opts())

	ctx, cancel := context.WithTimeout(context.Background(), waitFor)
	defer cancel()
	if err := c.Run(ctx); !errors.Is(err, dlqErr) {
		t.Fatalf("Run = %v, want the dlq error", err)
	}
	first.Close()

	second, err := kafkax.NewConsumerClient(kc, "g-dlqfail", []string{"orders"})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer second.Close()
	got := make(chan consume.Record, 1)
	c2 := consume.NewConsumer(second, func(_ context.Context, r consume.Record) error {
		select {
		case got <- r:
		default:
		}
		return nil
	}, &fakeDLQ{}, opts())
	ctx2, cancel2 := context.WithTimeout(context.Background(), waitFor)
	defer cancel2()
	done := make(chan error, 1)
	go func() { done <- c2.Run(ctx2) }()

	select {
	case r := <-got:
		if string(r.Key) != "k1" || string(r.Value) != "v1" || r.Topic != "orders" {
			t.Fatalf("redelivered = %+v", r)
		}
	case <-ctx2.Done():
		t.Fatal("record was not redelivered after a failed DLQ produce")
	}
	cancel2()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
}
