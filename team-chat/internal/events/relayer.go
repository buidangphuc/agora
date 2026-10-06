package events

import (
	"context"
	"log/slog"
	"time"

	"github.com/buidangphuc/team-chat/internal/repository"
)

// RelayerConfig tunes the background outbox polling loop.
type RelayerConfig struct {
	Topic        string        // default ChatEventsTopic
	PollInterval time.Duration // default 500ms
	BatchSize    int           // default 100
	LockDuration time.Duration // lease while producing a batch; default 30s
	MaxAttempts  int           // attempts before a row is parked 'failed'; default 10
	BaseBackoff  time.Duration // first retry delay, doubles per attempt; default 1s
	MaxBackoff   time.Duration // backoff ceiling; default 5m
}

func (c RelayerConfig) withDefaults() RelayerConfig {
	if c.PollInterval <= 0 {
		c.PollInterval = 500 * time.Millisecond
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 100
	}
	if c.LockDuration <= 0 {
		c.LockDuration = 30 * time.Second
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
	if c.Topic == "" {
		c.Topic = ChatEventsTopic
	}
	return c
}

// backoff returns the retry delay after attempt (1-based) failures.
func (c RelayerConfig) backoff(attempt int) time.Duration {
	d := c.BaseBackoff
	for i := 1; i < attempt && d < c.MaxBackoff; i++ {
		d *= 2
	}
	if d > c.MaxBackoff {
		d = c.MaxBackoff
	}
	return d
}

// Relayer continuously reads claimed outbox events and publishes them to Kafka.
type Relayer struct {
	repo      repository.OutboxRepository
	publisher KafkaPublisher
	cfg       RelayerConfig
	logger    *slog.Logger
}

// NewRelayer constructs a Relayer instance.
func NewRelayer(
	repo repository.OutboxRepository,
	publisher KafkaPublisher,
	cfg RelayerConfig,
	logger *slog.Logger,
) *Relayer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Relayer{
		repo:      repo,
		publisher: publisher,
		cfg:       cfg.withDefaults(),
		logger:    logger,
	}
}

// SweepClaims processes one batch of pending outbox events. Returns count processed.
func (r *Relayer) SweepClaims(ctx context.Context) (int, error) {
	events, err := r.repo.ClaimPending(ctx, r.cfg.BatchSize, r.cfg.LockDuration)
	if err != nil {
		return 0, err
	}
	if len(events) == 0 {
		return 0, nil
	}

	publishedCount := 0
	for _, ev := range events {
		if err := r.publisher.Publish(ctx, r.cfg.Topic, ev.AggregateID, ev.Payload); err != nil {
			attempt := ev.Attempts + 1
			if attempt >= r.cfg.MaxAttempts {
				r.logger.Error("outbox event parked after max attempts",
					"event_id", ev.EventID, "attempts", attempt, "err", err)
				if perr := r.repo.MarkParked(ctx, ev.EventID, err.Error()); perr != nil {
					r.logger.Error("failed to park outbox event", "event_id", ev.EventID, "err", perr)
				}
				continue
			}
			r.logger.Warn("failed to publish outbox event; will retry",
				"event_id", ev.EventID, "attempt", attempt, "err", err)
			if merr := r.repo.MarkFailed(ctx, ev.EventID, err.Error(), r.cfg.backoff(attempt)); merr != nil {
				r.logger.Error("failed to mark outbox event failed", "event_id", ev.EventID, "err", merr)
			}
			continue
		}
		if err := r.repo.MarkPublished(ctx, ev.EventID); err != nil {
			r.logger.Error("failed to mark outbox event published", "event_id", ev.EventID, "err", err)
			continue
		}
		publishedCount++
	}
	return publishedCount, nil
}

// Run starts the relayer background loop until context cancellation.
func (r *Relayer) Run(ctx context.Context) {
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := r.SweepClaims(ctx); err != nil {
				r.logger.Error("outbox sweep error", "err", err)
			}
		}
	}
}
