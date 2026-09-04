package bootstrap

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/team-payment/internal/events"
)

// kafkaConfig is the outbox-relayer's Kafka wiring, read from the environment.
// team-payment's Settings does not model Kafka, and its config package is outside
// this wave's write-set; these keys are registered in the compose/gitops env in
// the integration wave (Part B). Enabled gates the relayer.
type kafkaConfig struct {
	Enabled bool
	Brokers []string
	Topic   string
}

// kafkaConfigFromEnv reads the settle-event producer settings.
func kafkaConfigFromEnv() kafkaConfig {
	return kafkaConfig{
		Enabled: envBool("KAFKA_ENABLED", false),
		Brokers: envList("KAFKA_BROKERS", "localhost:9092"),
		Topic:   envStr("PAYMENT_EVENTS_TOPIC", events.PaymentEventsTopic),
	}
}

// kafkaProducer is a franz-go events.RawProducer: it produces already-marshalled
// EventEnvelope bytes to the payment.events topic, keyed by order_id for
// per-order ordering (AD4). It is the relayer's produce path — the outbox stored
// the whole envelope at enqueue time, so this stays agnostic of event specifics.
type kafkaProducer struct {
	client *kgo.Client
	topic  string
}

func newKafkaProducer(cfg kafkaConfig) (*kafkaProducer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ProducerLinger(0),
	)
	if err != nil {
		return nil, fmt.Errorf("kafka client: %w", err)
	}
	return &kafkaProducer{client: client, topic: cfg.Topic}, nil
}

// ProduceRaw produces the marshalled EventEnvelope to the configured topic.
func (p *kafkaProducer) ProduceRaw(ctx context.Context, key string, payload []byte) error {
	rec := &kgo.Record{Topic: p.topic, Key: []byte(key), Value: payload}
	if err := p.client.ProduceSync(ctx, rec).FirstErr(); err != nil {
		return fmt.Errorf("produce to %s: %w", p.topic, err)
	}
	return nil
}

// Close flushes and shuts the client down.
func (p *kafkaProducer) Close() { p.client.Close() }

var _ events.RawProducer = (*kafkaProducer)(nil)

// ── small env helpers (bootstrap-local; team-payment config has no Kafka group) ──

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
