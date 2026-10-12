package consume

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"
)

type nopDLQ struct{}

func (nopDLQ) PublishRecord(context.Context, *kgo.Record) error { return nil }

// logSink records the messages Run logs.
type logSink struct {
	mu   sync.Mutex
	msgs []string
}

func (l *logSink) Enabled(context.Context, slog.Level) bool { return true }
func (l *logSink) Handle(_ context.Context, r slog.Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.msgs = append(l.msgs, r.Message)
	return nil
}
func (l *logSink) WithAttrs([]slog.Attr) slog.Handler { return l }
func (l *logSink) WithGroup(string) slog.Handler      { return l }
func (l *logSink) has(msg string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, m := range l.msgs {
		if m == msg {
			return true
		}
	}
	return false
}

func TestFetchErrorDoesNotStopLoop(t *testing.T) {
	brokers := testkit.Redpanda(t)
	const topic = "orders"
	client, err := kafkax.NewConsumerClient(config.Kafka{Brokers: brokers}, "g-fetcherr", []string{topic},
		kgo.AllowAutoTopicCreation(), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer client.Close()

	sink := &logSink{}
	got := make(chan string, 1)
	c := NewConsumer(client, func(_ context.Context, r Record) error {
		got <- string(r.Value)
		return nil
	}, nopDLQ{}, Options{DLQTopic: "orders.dlq", Logger: slog.New(sink)})

	// The first poll reports a fetch error; later polls are the real client.
	var once sync.Once
	c.poll = func(ctx context.Context) kgo.Fetches {
		injected := false
		var f kgo.Fetches
		once.Do(func() {
			injected = true
			f = kgo.Fetches{{Topics: []kgo.FetchTopic{{
				Topic:      topic,
				Partitions: []kgo.FetchPartition{{Partition: 0, Err: errors.New("broker hiccup")}},
			}}}}
		})
		if injected {
			return f
		}
		return client.PollFetches(ctx)
	}

	const wait = 25 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()

	produceOne(t, brokers, topic, "later")
	select {
	case v := <-got:
		if v != "later" {
			t.Fatalf("handled %q, want later", v)
		}
	case err := <-done:
		t.Fatalf("Run returned %v after a fetch error", err)
	case <-time.After(wait):
		t.Fatal("record produced after the fetch error was not handled")
	}
	if !sink.has("consume: fetch error") {
		t.Error("fetch error was not logged")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func produceOne(t *testing.T, brokers, topic, value string) {
	t.Helper()
	p, err := kafkax.NewProducer(config.Kafka{Brokers: brokers}, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatalf("producer: %v", err)
	}
	defer p.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := p.Publish(ctx, topic, []byte("k"), []byte(value)); err != nil {
		t.Fatalf("produce: %v", err)
	}
}
