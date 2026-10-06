package bootstrap

import (
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/team-order/internal/config"
	"github.com/buidangphuc/team-order/internal/consumer"
	"github.com/buidangphuc/team-order/internal/events"
)

// KafkaConfig is the Kafka wiring for the payment.events consumer and the
// order.events producer. Enabled gates both.
type KafkaConfig struct {
	Enabled       bool
	Brokers       []string
	ConsumerGroup string
	Topic         string // payment.events (consumed)
	DLQTopic      string
	OrderTopic    string // order.events (produced by the outbox relayer)
}

// KafkaConfigFromSettings derives the Kafka wiring from the loaded Settings.
func KafkaConfigFromSettings(s *config.Settings) KafkaConfig {
	return KafkaConfig{
		Enabled:       s.Kafka.Enabled,
		Brokers:       s.KafkaBrokers(),
		ConsumerGroup: s.Kafka.ConsumerGroup,
		Topic:         s.Kafka.PaymentTopic,
		DLQTopic:      s.Kafka.PaymentDLQ,
		OrderTopic:    s.Kafka.OrderTopic,
	}
}

// OrderEventsProducer is a franz-go events.KafkaPublisher for the outbox relayer.
// The outbox stores the whole marshalled EventEnvelope, so it is produced as-is,
// keyed by order_id for per-order ordering.
type OrderEventsProducer struct {
	client *kgo.Client
}

// NewOrderEventsProducer dials the brokers for producing.
func NewOrderEventsProducer(cfg KafkaConfig) (*OrderEventsProducer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ProducerLinger(0),
	)
	if err != nil {
		return nil, fmt.Errorf("kafka client: %w", err)
	}
	return &OrderEventsProducer{client: client}, nil
}

// Publish produces payload to topic keyed by key and waits for the ack.
func (p *OrderEventsProducer) Publish(ctx context.Context, topic, key string, payload []byte) error {
	rec := &kgo.Record{Topic: topic, Key: []byte(key), Value: payload}
	if err := p.client.ProduceSync(ctx, rec).FirstErr(); err != nil {
		return fmt.Errorf("produce to %s: %w", topic, err)
	}
	return nil
}

// Close flushes and shuts the client down.
func (p *OrderEventsProducer) Close() { p.client.Close() }

var _ events.KafkaPublisher = (*OrderEventsProducer)(nil)

// PaymentKafka holds the franz-go handles backing the PaymentSettled consumer: a
// consumer-group reader on payment.events with auto-commit DISABLED (so offsets
// advance only after a record is applied or DLQ'd, AD1) and a producer for the
// DLQ topic.
type PaymentKafka struct {
	reader *kafkaReader
	dlq    *kafkaDLQ
}

// NewPaymentKafka dials the brokers and joins the consumer group.
func NewPaymentKafka(cfg KafkaConfig) (*PaymentKafka, error) {
	consumerClient, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ConsumerGroup(cfg.ConsumerGroup),
		kgo.ConsumeTopics(cfg.Topic),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return nil, err
	}
	dlqClient, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ProducerLinger(0),
	)
	if err != nil {
		consumerClient.Close()
		return nil, err
	}
	return &PaymentKafka{
		reader: &kafkaReader{client: consumerClient},
		dlq:    &kafkaDLQ{client: dlqClient},
	}, nil
}

// Reader returns the consumer.RecordReader over payment.events.
func (k *PaymentKafka) Reader() consumer.RecordReader { return k.reader }

// DLQ returns the consumer.DeadLetterProducer for parked records.
func (k *PaymentKafka) DLQ() consumer.DeadLetterProducer { return k.dlq }

// Close flushes and shuts both clients down.
func (k *PaymentKafka) Close() {
	k.reader.client.Close()
	k.dlq.client.Close()
}

// kafkaReader adapts a franz-go consumer-group client to consumer.RecordReader.
// The consume loop is strictly sequential (Fetch -> process -> Commit), so the
// reader buffers one poll's records, hands them out one at a time, and tracks the
// underlying record so Commit advances the group offset exactly past it.
type kafkaReader struct {
	client  *kgo.Client
	pending []*kgo.Record
	cur     *kgo.Record
}

// Fetch returns the next record, polling a fresh batch when the buffer is empty.
// It blocks until a record is available or ctx is cancelled.
func (r *kafkaReader) Fetch(ctx context.Context) (consumer.Record, error) {
	for len(r.pending) == 0 {
		if err := ctx.Err(); err != nil {
			return consumer.Record{}, err
		}
		fetches := r.client.PollFetches(ctx)
		if errs := fetches.Errors(); len(errs) > 0 {
			// Surface the first error; the loop returns on context errors and logs
			// and retries transient ones.
			return consumer.Record{}, errs[0].Err
		}
		fetches.EachRecord(func(rec *kgo.Record) {
			r.pending = append(r.pending, rec)
		})
	}
	rec := r.pending[0]
	r.pending = r.pending[1:]
	r.cur = rec
	return consumer.Record{Key: string(rec.Key), Value: rec.Value}, nil
}

// Commit advances the committed offset past the record last returned by Fetch.
func (r *kafkaReader) Commit(ctx context.Context, _ consumer.Record) error {
	if r.cur == nil {
		return nil
	}
	return r.client.CommitRecords(ctx, r.cur)
}

// kafkaDLQ produces poison / max-retried records to a dead-letter topic (AD1).
type kafkaDLQ struct {
	client *kgo.Client
}

func (d *kafkaDLQ) Produce(ctx context.Context, topic string, rec consumer.Record) error {
	kr := &kgo.Record{Topic: topic, Key: []byte(rec.Key), Value: rec.Value}
	return d.client.ProduceSync(ctx, kr).FirstErr()
}
