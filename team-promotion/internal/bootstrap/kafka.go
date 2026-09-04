package bootstrap

import (
	"context"

	"github.com/twmb/franz-go/pkg/kgo"
)

// EventProducer publishes promotion state-change events (voucher/campaign
// create/update) to the promotion.events topic (ADR-0002), wrapped by callers in
// an EventEnvelope. It is a thin franz-go wrapper; the concrete emit helpers land
// with the Wave 1 business logic (internal/producer). Kept here so the resource
// lifecycle (dial on boot, flush on shutdown) mirrors team-order's Kafka wiring.
type EventProducer struct {
	client *kgo.Client
	topic  string
}

// NewEventProducer dials the brokers and returns a producer bound to the
// promotion.events topic. Auto-linger is disabled so a state-change event is
// flushed promptly rather than batched behind a timer.
func NewEventProducer(brokers []string, topic string) (*EventProducer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.DefaultProduceTopic(topic),
		kgo.ProducerLinger(0),
	)
	if err != nil {
		return nil, err
	}
	return &EventProducer{client: client, topic: topic}, nil
}

// Topic returns the configured promotion.events topic.
func (p *EventProducer) Topic() string { return p.topic }

// Publish produces a single keyed record synchronously to promotion.events.
func (p *EventProducer) Publish(ctx context.Context, key string, value []byte) error {
	rec := &kgo.Record{Topic: p.topic, Key: []byte(key), Value: value}
	return p.client.ProduceSync(ctx, rec).FirstErr()
}

// Close flushes buffered records and shuts the client down.
func (p *EventProducer) Close() {
	if p == nil || p.client == nil {
		return
	}
	p.client.Close()
}
