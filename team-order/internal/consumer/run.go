package consumer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// ── Transport loop (AD1 offset discipline) ──

// ErrPermanent marks an error that will never succeed on retry (a poison record).
// The loop routes such records to the DLQ instead of retrying indefinitely.
var ErrPermanent = errors.New("permanent consumer error")

// Record is one Kafka record the transport hands to the loop.
type Record struct {
	Key   string
	Value []byte
}

// RecordReader is the transport the loop reads from. A real franz-go reader
// satisfies it; tests use a fake. Fetch blocks until the next record (or ctx is
// done); Commit advances the committed offset past rec.
type RecordReader interface {
	Fetch(ctx context.Context) (Record, error)
	Commit(ctx context.Context, rec Record) error
}

// DeadLetterProducer publishes poison / max-retried records to a DLQ topic (AD1).
type DeadLetterProducer interface {
	Produce(ctx context.Context, topic string, rec Record) error
}

// RunConfig tunes the consume loop.
type RunConfig struct {
	DLQTopic    string        // e.g. "payment.events.dlq"
	MaxAttempts int           // in-process attempts before DLQ (default 5)
	BaseBackoff time.Duration // first retry delay; doubles per attempt (default 200ms)
	// FetchRetryBackoff spaces out retries after a failed fetch (default 1s).
	FetchRetryBackoff time.Duration
	// MaxBackoff caps the retry delay (default: uncapped, or 5s when
	// RetryTransientUntilApplied is set).
	MaxBackoff time.Duration
	// RetryTransientUntilApplied keeps retrying a transient failure forever
	// instead of dead-lettering after MaxAttempts; only permanent errors reach
	// the DLQ.
	RetryTransientUntilApplied bool
}

func (c RunConfig) withDefaults() RunConfig {
	if c.DLQTopic == "" {
		c.DLQTopic = "payment.events.dlq"
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 5
	}
	if c.BaseBackoff <= 0 {
		c.BaseBackoff = 200 * time.Millisecond
	}
	if c.FetchRetryBackoff <= 0 {
		c.FetchRetryBackoff = time.Second
	}
	if c.RetryTransientUntilApplied && c.MaxBackoff <= 0 {
		c.MaxBackoff = 5 * time.Second
	}
	return c
}

// runLoop consumes until ctx is cancelled. Per record it applies handle with
// bounded in-process retries; on a permanent error or once retries are exhausted
// it produces the record to the DLQ. Crucially it commits the offset ONLY after
// the record is either applied or DLQ'd - it never advances past an unprocessed
// record (AD1).
func runLoop(ctx context.Context, logger *slog.Logger, handle func(context.Context, []byte) error, reader RecordReader, dlq DeadLetterProducer, cfg RunConfig) error {
	cfg = cfg.withDefaults()
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rec, err := reader.Fetch(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.WarnContext(ctx, "fetch record failed; retrying", slog.Any("err", err))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(cfg.FetchRetryBackoff):
			}
			continue
		}

		if perr := processWithRetry(ctx, logger, handle, rec, dlq, cfg); perr != nil {
			logger.ErrorContext(ctx, "record neither applied nor DLQ'd; not committing",
				slog.Any("err", perr))
			continue
		}
		if err := reader.Commit(ctx, rec); err != nil {
			logger.WarnContext(ctx, "commit offset failed; record may redeliver", slog.Any("err", err))
		}
	}
}

func retryDelay(cfg RunConfig, attempt int) time.Duration {
	d := cfg.BaseBackoff * time.Duration(attempt)
	if cfg.MaxBackoff > 0 && (d > cfg.MaxBackoff || d <= 0) {
		d = cfg.MaxBackoff
	}
	return d
}

func processWithRetry(ctx context.Context, logger *slog.Logger, handle func(context.Context, []byte) error, rec Record, dlq DeadLetterProducer, cfg RunConfig) error {
	var lastErr error
	for attempt := 1; cfg.RetryTransientUntilApplied || attempt <= cfg.MaxAttempts; attempt++ {
		err := handle(ctx, rec.Value)
		if err == nil {
			return nil
		}
		lastErr = err
		if errors.Is(err, ErrPermanent) {
			break
		}
		if cfg.RetryTransientUntilApplied || attempt < cfg.MaxAttempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryDelay(cfg, attempt)):
			}
		}
	}
	logger.WarnContext(ctx, "routing record to DLQ",
		slog.String("dlq_topic", cfg.DLQTopic), slog.Any("err", lastErr))
	if err := dlq.Produce(ctx, cfg.DLQTopic, rec); err != nil {
		return fmt.Errorf("produce to DLQ: %w (original: %v)", err, lastErr)
	}
	return nil
}
