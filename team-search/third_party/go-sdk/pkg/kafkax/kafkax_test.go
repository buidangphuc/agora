package kafkax_test

import (
	"context"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"
)

func header(r *kgo.Record, key string) (string, bool) {
	for _, h := range r.Headers {
		if h.Key == key {
			return string(h.Value), true
		}
	}
	return "", false
}

func TestPublishAddsTraceAndRequestHeaders(t *testing.T) {
	brokers := testkit.Redpanda(t)

	otel.SetTextMapPropagator(propagation.TraceContext{})
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	otel.SetTracerProvider(tp)

	ctx, span := tp.Tracer("test").Start(context.Background(), "publish")
	defer span.End()
	ctx = reqctx.WithRequestID(ctx, "r1")

	p, err := kafkax.NewProducer(config.Kafka{Brokers: brokers}, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatalf("NewProducer: %v", err)
	}
	defer p.Close()

	const topic = "kafkax.headers"
	if err := p.Publish(ctx, topic, []byte("k1"), []byte("v1")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	reader, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatalf("reader: %v", err)
	}
	defer reader.Close()

	pollCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fetches := reader.PollFetches(pollCtx)
	if err := fetches.Err(); err != nil {
		t.Fatalf("poll: %v", err)
	}
	recs := fetches.Records()
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1", len(recs))
	}
	rec := recs[0]
	if string(rec.Key) != "k1" || string(rec.Value) != "v1" {
		t.Errorf("record = %q/%q", rec.Key, rec.Value)
	}
	if got, _ := header(rec, "x-request-id"); got != "r1" {
		t.Errorf("x-request-id = %q, want r1", got)
	}
	tp0, ok := header(rec, "traceparent")
	if !ok {
		t.Fatal("traceparent header missing")
	}
	// version-traceid-spanid-flags; the producer span is a child of ours.
	if want := span.SpanContext().TraceID().String(); len(tp0) != 55 || tp0[3:35] != want {
		t.Errorf("traceparent = %q, want trace id %s", tp0, want)
	}
}

func TestPublishReturnsErrorWhenBrokerDown(t *testing.T) {
	p, err := kafkax.NewProducer(config.Kafka{Brokers: "127.0.0.1:1"},
		kgo.RecordDeliveryTimeout(time.Second))
	if err != nil {
		t.Fatalf("NewProducer: %v", err)
	}
	defer p.Close()

	start := time.Now()
	err = p.Publish(context.Background(), "kafkax.down", []byte("k"), []byte("v"))
	if err == nil {
		t.Fatal("Publish returned nil with the broker down")
	}
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Errorf("Publish took %v, want a quick failure", elapsed)
	}
}

func TestPublishRecordKeepsCallerHeaders(t *testing.T) {
	brokers := testkit.Redpanda(t)

	p, err := kafkax.NewProducer(config.Kafka{Brokers: brokers}, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatalf("NewProducer: %v", err)
	}
	defer p.Close()

	ctx := reqctx.WithRequestID(context.Background(), "r2")
	const topic = "kafkax.record"
	rec := &kgo.Record{
		Topic: topic, Key: []byte("k"), Value: []byte("v"),
		Headers: []kgo.RecordHeader{{Key: "error", Value: []byte("boom")}},
	}
	if err := p.PublishRecord(ctx, rec); err != nil {
		t.Fatalf("PublishRecord: %v", err)
	}

	reader, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatalf("reader: %v", err)
	}
	defer reader.Close()
	pollCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	recs := reader.PollFetches(pollCtx).Records()
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1", len(recs))
	}
	if got, _ := header(recs[0], "error"); got != "boom" {
		t.Errorf("error header = %q, want boom", got)
	}
	if got, _ := header(recs[0], "x-request-id"); got != "r2" {
		t.Errorf("x-request-id = %q, want r2", got)
	}
}

func TestConsumerClientAcceptsOptions(t *testing.T) {
	brokers := testkit.Redpanda(t)
	kc := config.Kafka{Brokers: brokers}
	const topic = "kafkax.autocreate"

	client, err := kafkax.NewConsumerClient(kc, "g-opts", []string{topic},
		kgo.AllowAutoTopicCreation(), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatalf("NewConsumerClient: %v", err)
	}
	defer client.Close()

	p, err := kafkax.NewProducer(kc, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatalf("NewProducer: %v", err)
	}
	defer p.Close()
	if err := p.Publish(context.Background(), topic, []byte("k"), []byte("v")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var got []*kgo.Record
	for len(got) == 0 {
		f := client.PollFetches(ctx)
		if ctx.Err() != nil {
			t.Fatalf("no record before timeout: %v", f.Errors())
		}
		got = f.Records()
	}
	// The client blocks rebalances after a poll; Close waits for them to be allowed.
	client.AllowRebalance()
	if string(got[0].Value) != "v" {
		t.Errorf("value = %q, want v", got[0].Value)
	}
}

func TestGroupLessConsumerClient(t *testing.T) {
	brokers := testkit.Redpanda(t)
	cfg := config.Kafka{Brokers: brokers}

	p, err := kafkax.NewProducer(cfg, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatalf("NewProducer: %v", err)
	}
	defer p.Close()

	const topic = "kafkax.groupless"
	if err := p.Publish(context.Background(), topic, []byte("k1"), []byte("v1")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	c, err := kafkax.NewConsumerClient(cfg, "", []string{topic})
	if err != nil {
		t.Fatalf("NewConsumerClient: %v", err)
	}
	defer c.Close()

	pollCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fetches := c.PollFetches(pollCtx)
	if err := fetches.Err(); err != nil {
		t.Fatalf("poll: %v", err)
	}
	recs := fetches.Records()
	if len(recs) != 1 || string(recs[0].Value) != "v1" {
		t.Fatalf("records = %d, want the one published from the start", len(recs))
	}

	adm := kadm.NewClient(c)
	groups, err := adm.ListGroups(context.Background())
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if got := groups.Groups(); len(got) != 0 {
		t.Fatalf("consumer groups on the broker = %v, want none", got)
	}
}
