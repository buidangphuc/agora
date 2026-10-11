package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/team-payment/internal/consumer"
)

// StartSettlementConsumer starts the order.events -> seller-ledger consumer in its own
// goroutine and returns a stop function that cancels it, waits for it to return and
// closes its clients. Call stop before the DB pool closes. It is a no-op (nil-safe stop)
// unless KAFKA_ENABLED and PAYMENT_SETTLEMENT_CONSUMER_ENABLED are true and haveDB is true
// (the ledger needs the DB).
func StartSettlementConsumer(apply consumer.Applier, haveDB bool, logger *slog.Logger) (stop func(), err error) {
	cfg := kafkaConfigFromEnv()
	if !cfg.Enabled || !cfg.ConsumerEnabled || !haveDB {
		logger.Info("settlement consumer disabled",
			slog.Bool("kafka_enabled", cfg.Enabled), slog.Bool("consumer_enabled", cfg.ConsumerEnabled), slog.Bool("db", haveDB))
		return func() {}, nil
	}
	reader, dlq, closeFn, err := newSettlementKafka(cfg)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	c := consumer.NewSettlementConsumer(apply, logger)
	go func() {
		defer close(done)
		if err := c.Run(ctx, reader, dlq, consumer.RunConfig{DLQTopic: cfg.DLQTopic}); err != nil &&
			!errors.Is(err, context.Canceled) {
			logger.Error("settlement consumer stopped", slog.Any("err", err))
		}
	}()
	logger.Info("settlement consumer started",
		slog.String("topic", cfg.OrderEventsTopic), slog.String("group", cfg.ConsumerGroup), slog.String("dlq", cfg.DLQTopic))
	return func() {
		cancel()
		<-done
		closeFn()
	}, nil
}

// newSettlementKafka dials a consumer-group reader on order.events (auto-commit disabled:
// offsets move only after apply or DLQ) and a DLQ producer. A group with no committed
// offset starts at the latest offset (design D10): historical OrderPaidEvents were
// credited inline by the old binary and must not be credited again.
func newSettlementKafka(cfg kafkaConfig) (consumer.RecordReader, consumer.DeadLetterProducer, func(), error) {
	rc, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ConsumerGroup(cfg.ConsumerGroup),
		kgo.ConsumeTopics(cfg.OrderEventsTopic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("settlement consumer client: %w", err)
	}
	dc, err := kgo.NewClient(kgo.SeedBrokers(cfg.Brokers...), kgo.ProducerLinger(0))
	if err != nil {
		rc.Close()
		return nil, nil, nil, fmt.Errorf("settlement dlq client: %w", err)
	}
	return &kafkaReader{client: rc}, &kafkaDLQ{client: dc}, func() { rc.Close(); dc.Close() }, nil
}

// kafkaReader adapts a franz-go group client to consumer.RecordReader. The loop is
// strictly sequential (Fetch -> process -> Commit): it buffers one poll's records and
// tracks the current one so Commit advances the group offset exactly past it.
type kafkaReader struct {
	client  *kgo.Client
	pending []*kgo.Record
	cur     *kgo.Record
}

func (r *kafkaReader) Fetch(ctx context.Context) (consumer.Record, error) {
	for len(r.pending) == 0 {
		if err := ctx.Err(); err != nil {
			return consumer.Record{}, err
		}
		fetches := r.client.PollFetches(ctx)
		// Keep the records a poll returned even when some partition also reported an
		// error: franz-go has already moved past them, so dropping them here and later
		// committing a newer record would lose them.
		fetches.EachRecord(func(rec *kgo.Record) { r.pending = append(r.pending, rec) })
		if errs := fetches.Errors(); len(errs) > 0 && len(r.pending) == 0 {
			return consumer.Record{}, errs[0].Err
		}
	}
	rec := r.pending[0]
	r.pending = r.pending[1:]
	r.cur = rec
	return consumer.Record{Key: string(rec.Key), Value: rec.Value}, nil
}

func (r *kafkaReader) Commit(ctx context.Context, _ consumer.Record) error {
	if r.cur == nil {
		return nil
	}
	return r.client.CommitRecords(ctx, r.cur)
}

type kafkaDLQ struct{ client *kgo.Client }

func (d *kafkaDLQ) Produce(ctx context.Context, topic string, rec consumer.Record) error {
	return d.client.ProduceSync(ctx, &kgo.Record{Topic: topic, Key: []byte(rec.Key), Value: rec.Value}).FirstErr()
}
