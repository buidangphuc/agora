// Package events relays the transactional outbox to Kafka (engagement-fact-events D1).
// Shape copied from team-order's relayer: poll, publish in seq order, mark published.
package events

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/buidangphuc/team-engagement/generated/platform/events/v1"
	"github.com/buidangphuc/team-engagement/internal/repository"
)

// DefaultTopic is the Kafka topic carrying engagement facts.
const DefaultTopic = "engagement.events"

// Publisher sends one keyed record to a topic.
type Publisher interface {
	Publish(ctx context.Context, topic, key string, value []byte) error
}

// Store is the outbox the relayer drains (repository.PgOutbox).
type Store interface {
	Relay(ctx context.Context, batch int, retention time.Duration,
		publish func(context.Context, repository.OutboxMessage) error) (int, error)
}

type RelayerConfig struct {
	Topic        string        // default engagement.events
	PollInterval time.Duration // default 500ms
	BatchSize    int           // default 100
	Retention    time.Duration // published rows older than this are deleted; default 7 days
}

func (c RelayerConfig) withDefaults() RelayerConfig {
	if c.Topic == "" {
		c.Topic = DefaultTopic
	}
	if c.PollInterval <= 0 {
		c.PollInterval = 500 * time.Millisecond
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 100
	}
	if c.Retention <= 0 {
		c.Retention = repository.OutboxRetention
	}
	return c
}

type Relayer struct {
	store  Store
	pub    Publisher
	cfg    RelayerConfig
	logger *slog.Logger
}

func NewRelayer(store Store, pub Publisher, cfg RelayerConfig, logger *slog.Logger) *Relayer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Relayer{store: store, pub: pub, cfg: cfg.withDefaults(), logger: logger}
}

// BuildEnvelope wraps an outbox row in the standard EventEnvelope: the principal
// is the caller recorded with the row and type is the payload's full name.
func BuildEnvelope(m repository.OutboxMessage) ([]byte, error) {
	b, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId:    m.EventID,
		Type:       m.Type,
		OccurredAt: timestamppb.New(m.OccurredAt),
		Principal:  m.PrincipalOf(),
		RequestId:  m.RequestID,
		Payload:    m.Payload,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	return b, nil
}

// Sweep runs one relay cycle and returns the number of facts published.
func (r *Relayer) Sweep(ctx context.Context) (int, error) {
	return r.store.Relay(ctx, r.cfg.BatchSize, r.cfg.Retention, func(ctx context.Context, m repository.OutboxMessage) error {
		v, err := BuildEnvelope(m)
		if err != nil {
			return err
		}
		return r.pub.Publish(ctx, r.cfg.Topic, m.Key, v)
	})
}

// Run polls until ctx is cancelled.
func (r *Relayer) Run(ctx context.Context) {
	t := time.NewTicker(r.cfg.PollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := r.Sweep(ctx); err != nil && ctx.Err() == nil {
				r.logger.Warn("outbox sweep failed; will retry", slog.Any("err", err))
			}
		}
	}
}
