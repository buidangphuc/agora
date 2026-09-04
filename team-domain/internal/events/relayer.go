package events

import (
	"context"
	"log/slog"
	"time"

	"github.com/buidangphuc/team-domain/internal/repository"
)

// OutboxClaimer is the slice of the outbox store the relayer needs: claim a
// batch of pending rows, then mark each published or failed. Satisfied by
// *repository.OutboxStore (Postgres) and by in-memory fakes in tests.
type OutboxClaimer interface {
	ClaimPending(ctx context.Context, batch, lockSeconds int) ([]repository.PendingEvent, error)
	MarkPublished(ctx context.Context, eventID string, publishedAt time.Time) error
	MarkFailed(ctx context.Context, eventID, reason string, availableAt *time.Time) error
}

// RawProducer produces already-marshalled EventEnvelope bytes keyed for
// ordering. Satisfied by *KafkaPublisher.
type RawProducer interface {
	ProduceRaw(ctx context.Context, key string, payload []byte) error
}

// RelayerConfig tunes the poll loop. Zero fields fall back to sane defaults.
type RelayerConfig struct {
	PollInterval time.Duration // how often to poll for pending rows (default 1s)
	BatchSize    int           // rows claimed per poll (default 100)
	LockSeconds  int           // lease held while producing a batch (default 60)
	MaxAttempts  int           // attempts before a row is parked 'failed' (default 10)
	BaseBackoff  time.Duration // first retry delay; doubles per attempt (default 1s)
	MaxBackoff   time.Duration // backoff ceiling (default 5m)
}

func (c RelayerConfig) withDefaults() RelayerConfig {
	if c.PollInterval <= 0 {
		c.PollInterval = time.Second
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 100
	}
	if c.LockSeconds <= 0 {
		c.LockSeconds = 60
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 10
	}
	if c.BaseBackoff <= 0 {
		c.BaseBackoff = time.Second
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = 5 * time.Minute
	}
	return c
}

// Relayer drains the transactional outbox: it periodically claims pending rows
// and produces each stored EventEnvelope to Kafka, marking rows published on
// success and retrying (with backoff) on failure. Delivery is AT-LEAST-ONCE — a
// row is marked published only after a successful produce, so a crash between
// produce and mark re-delivers the row (consumers dedupe on event_id). Mirrors
// team-ai's OutboxPublisher.publish_pending.
type Relayer struct {
	store    OutboxClaimer
	producer RawProducer
	logger   *slog.Logger
	cfg      RelayerConfig
}

// NewRelayer builds a relayer over the outbox store and producer.
func NewRelayer(store OutboxClaimer, producer RawProducer, logger *slog.Logger, cfg RelayerConfig) *Relayer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Relayer{store: store, producer: producer, logger: logger, cfg: cfg.withDefaults()}
}

// Report summarises one relay pass, for tests and observability.
type Report struct {
	Claimed   int
	Published int
	Retried   int
	Failed    int
}

// Run polls on PollInterval until ctx is cancelled. It is meant to run in its
// own goroutine; return signals a clean stop.
func (r *Relayer) Run(ctx context.Context) {
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := r.RelayOnce(ctx, time.Now()); err != nil {
				// A claim error (e.g. DB blip) is transient; log and retry next tick.
				r.logger.WarnContext(ctx, "outbox relay pass failed", slog.Any("err", err))
			}
		}
	}
}

// RelayOnce claims one batch and produces it, returning a Report. It is exported
// so tests can drive a single deterministic pass without the ticker.
func (r *Relayer) RelayOnce(ctx context.Context, now time.Time) (Report, error) {
	rows, err := r.store.ClaimPending(ctx, r.cfg.BatchSize, r.cfg.LockSeconds)
	if err != nil {
		return Report{}, err
	}
	rep := Report{Claimed: len(rows)}
	for _, ev := range rows {
		if err := r.producer.ProduceRaw(ctx, ev.AggregateID, ev.Payload); err != nil {
			r.handleFailure(ctx, ev, now, err, &rep)
			continue
		}
		if err := r.store.MarkPublished(ctx, ev.EventID, now); err != nil {
			// Produce succeeded but the mark failed; the row stays claimable and
			// will be re-delivered (at-least-once). Log and move on.
			r.logger.WarnContext(ctx, "outbox mark published failed",
				slog.String("event_id", ev.EventID), slog.Any("err", err))
			continue
		}
		rep.Published++
	}
	return rep, nil
}

// handleFailure schedules a retry with exponential backoff, or parks the row in
// 'failed' once it exceeds MaxAttempts — without blocking the rest of the batch.
func (r *Relayer) handleFailure(ctx context.Context, ev repository.PendingEvent, now time.Time, produceErr error, rep *Report) {
	attempt := ev.Attempts + 1
	if attempt >= r.cfg.MaxAttempts {
		if err := r.store.MarkFailed(ctx, ev.EventID, produceErr.Error(), nil); err != nil {
			r.logger.WarnContext(ctx, "outbox mark failed (parked) errored",
				slog.String("event_id", ev.EventID), slog.Any("err", err))
			return
		}
		rep.Failed++
		r.logger.WarnContext(ctx, "outbox event parked after max attempts",
			slog.String("event_id", ev.EventID), slog.Int("attempts", attempt), slog.Any("err", produceErr))
		return
	}
	next := now.Add(r.backoff(attempt))
	if err := r.store.MarkFailed(ctx, ev.EventID, produceErr.Error(), &next); err != nil {
		r.logger.WarnContext(ctx, "outbox mark failed (retry) errored",
			slog.String("event_id", ev.EventID), slog.Any("err", err))
		return
	}
	rep.Retried++
}

// backoff returns the delay before the next attempt: BaseBackoff doubled once
// per prior attempt, capped at MaxBackoff. attempt is 1-based.
func (r *Relayer) backoff(attempt int) time.Duration {
	d := r.cfg.BaseBackoff
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= r.cfg.MaxBackoff {
			return r.cfg.MaxBackoff
		}
	}
	return d
}
