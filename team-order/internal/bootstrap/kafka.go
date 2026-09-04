package bootstrap

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/team-order/internal/consumer"
)

// KafkaConfig is the payment-events consumer wiring, read from the environment.
// team-order's Settings does not model Kafka, and its config package is outside
// this wave's write-set; these keys are registered in the compose/gitops env in
// the integration wave (Part B). Enabled gates the whole consumer.
type KafkaConfig struct {
	Enabled       bool
	Brokers       []string
	ConsumerGroup string
	Topic         string
	DLQTopic      string
}

// KafkaConfigFromEnv reads the PaymentSettled consumer settings, applying the
// same defaults the topic/DLQ conventions expect (AD1/AD4).
func KafkaConfigFromEnv() KafkaConfig {
	return KafkaConfig{
		Enabled:       envBool("KAFKA_ENABLED", false),
		Brokers:       envList("KAFKA_BROKERS", "localhost:9092"),
		ConsumerGroup: envStr("ORDER_PAYMENT_CONSUMER_GROUP", "team-order.payment"),
		Topic:         envStr("PAYMENT_EVENTS_TOPIC", "payment.events"),
		DLQTopic:      envStr("PAYMENT_EVENTS_DLQ_TOPIC", "payment.events.dlq"),
	}
}

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

// ── small env helpers (bootstrap-local; team-order config has no Kafka group) ──

func envStr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
			return b
		}
	}
	return def
}

func envList(key, def string) []string {
	raw := envStr(key, def)
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
