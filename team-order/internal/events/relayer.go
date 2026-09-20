package events

import (
	"context"
	"log/slog"
	"time"

	"github.com/buidangphuc/team-order/internal/repository"
)

// RelayerConfig tunes the background outbox polling loop.
type RelayerConfig struct {
	PollInterval time.Duration
	BatchSize    int
	LockDuration time.Duration
	RetryDelay   time.Duration
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
	if c.RetryDelay <= 0 {
		c.RetryDelay = 5 * time.Second
	}
	return c
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
		if err := r.publisher.Publish(ctx, OrderEventsTopic, ev.AggregateID, ev.Payload); err != nil {
			r.logger.Error("failed to publish outbox event", "event_id", ev.EventID, "err", err)
			_ = r.repo.MarkFailed(ctx, ev.EventID, err.Error(), r.cfg.RetryDelay)
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
