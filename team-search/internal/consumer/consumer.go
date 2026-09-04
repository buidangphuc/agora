// Package consumer runs the Kafka indexer: it reads listing events and keeps the
// OpenSearch read-model up to date (ADR-0002 / ADR-0005).
package consumer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Handler processes one event record. On error the record is retried with
// bounded backoff and, if still failing, parked to the DLQ before the offset is
// allowed to advance (AD1) — never silently dropped.
type Handler func(ctx context.Context, key, value []byte) error

// DLQProducer parks a record that has exhausted retries onto the dead-letter
// topic. Satisfied by *kgoDLQ and by fakes in tests.
type DLQProducer interface {
	Produce(ctx context.Context, key, value []byte) error
	Close()
}

// RetryConfig tunes the per-record retry/park loop (AD1). Zero fields fall back
// to sane defaults, mirroring team-domain's relayer park-after-max-attempts.
type RetryConfig struct {
	MaxAttempts int           // handler attempts before a record is parked to DLQ (default 5)
	BaseBackoff time.Duration // first retry delay; doubles per attempt (default 100ms)
	MaxBackoff  time.Duration // backoff ceiling (default 30s)
}

func (c RetryConfig) withDefaults() RetryConfig {
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 5
	}
	if c.BaseBackoff <= 0 {
		c.BaseBackoff = 100 * time.Millisecond
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = 30 * time.Second
	}
	return c
}

// backoff returns the delay before the next attempt: BaseBackoff doubled once per
// prior attempt, capped at MaxBackoff. attempt is 1-based.
func (c RetryConfig) backoff(attempt int) time.Duration {
	d := c.BaseBackoff
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= c.MaxBackoff {
			return c.MaxBackoff
		}
	}
	return d
}

// Consumer is a franz-go consumer-group client bound to one topic, with
// auto-commit disabled so offsets advance only after a record is durably handled
// (or parked to the DLQ).
type Consumer struct {
	client *kgo.Client
	dlq    DLQProducer
	cfg    RetryConfig
}

// kgoDLQ produces parked records to the dead-letter topic over its own client.
type kgoDLQ struct {
	client *kgo.Client
	topic  string
}

func (d *kgoDLQ) Produce(ctx context.Context, key, value []byte) error {
	rec := &kgo.Record{Topic: d.topic, Key: key, Value: value}
	if err := d.client.ProduceSync(ctx, rec).FirstErr(); err != nil {
		return fmt.Errorf("produce to %s: %w", d.topic, err)
	}
	return nil
}

func (d *kgoDLQ) Close() { d.client.Close() }

// New dials the brokers and joins the consumer group for topic. Auto-commit is
// disabled: Run commits explicitly only after each record is handled or parked.
// The DLQ topic follows the "<topic>.dlq" convention (AD1).
func New(brokers []string, group, topic string) (*Consumer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return nil, err
	}
	dlqClient, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ProducerLinger(0),
	)
	if err != nil {
		client.Close()
		return nil, err
	}
	return &Consumer{
		client: client,
		dlq:    &kgoDLQ{client: dlqClient, topic: topic + ".dlq"},
		cfg:    RetryConfig{}.withDefaults(),
	}, nil
}

// Run polls and dispatches records until ctx is cancelled. Offset discipline
// (AD1): a record's offset is committed ONLY after the handler succeeds or, after
// bounded retries, the record is parked to the DLQ. If a record can neither be
// handled nor parked (e.g. DLQ unreachable), Run returns the error WITHOUT
// committing, so a restart reprocesses from the last commit rather than losing
// the event.
func (c *Consumer) Run(ctx context.Context, h Handler, logger *slog.Logger) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		fetches := c.client.PollFetches(ctx)
		if errs := fetches.Errors(); len(errs) > 0 {
			for _, e := range errs {
				if errors.Is(e.Err, context.Canceled) {
					return nil
				}
			}
			// Non-fatal fetch errors: log and retry the loop.
			logger.Warn("kafka fetch error", slog.Any("errs", fetches.Errors()))
			continue
		}
		iter := fetches.RecordIter()
		for !iter.Done() {
			rec := iter.Next()
			if err := c.processRecord(ctx, rec.Key, rec.Value, h, logger); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				// Unresolvable record: do NOT commit past it (never lose an event).
				return err
			}
		}
		if err := c.client.CommitUncommittedOffsets(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			logger.Warn("commit offsets failed", slog.Any("err", err))
		}
	}
}

// processRecord runs the handler with bounded exponential-backoff retries; on
// exhaustion it parks the record to the DLQ (AD1). It returns nil once the record
// is resolved (handled or parked) and its offset may advance, and a non-nil error
// only when the record could not be parked and the offset must NOT advance.
func (c *Consumer) processRecord(ctx context.Context, key, value []byte, h Handler, logger *slog.Logger) error {
	var lastErr error
	for attempt := 1; attempt <= c.cfg.MaxAttempts; attempt++ {
		if lastErr = h(ctx, key, value); lastErr == nil {
			return nil
		}
		logger.Warn("event handler failed",
			slog.Int("attempt", attempt),
			slog.String("key", string(key)),
			slog.Any("err", lastErr),
		)
		if attempt < c.cfg.MaxAttempts {
			if !sleepCtx(ctx, c.cfg.backoff(attempt)) {
				return ctx.Err() // cancelled mid-retry: do not advance.
			}
		}
	}
	// Retries exhausted → park to DLQ, then allow the offset to advance.
	if err := c.dlq.Produce(ctx, key, value); err != nil {
		logger.Error("dlq produce failed; offset will not advance",
			slog.String("key", string(key)),
			slog.Any("err", err),
		)
		return fmt.Errorf("dlq produce: %w", err)
	}
	logger.Warn("event parked to dlq after max attempts",
		slog.String("key", string(key)),
		slog.Int("attempts", c.cfg.MaxAttempts),
		slog.Any("err", lastErr),
	)
	return nil
}

// sleepCtx sleeps for d, returning false if ctx is cancelled first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// Close shuts the consumer and DLQ producer down.
func (c *Consumer) Close() {
	c.client.Close()
	if c.dlq != nil {
		c.dlq.Close()
	}
}
