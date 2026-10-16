package consumer

// TEST-STUB: compile-only placeholder so identity_test.go builds and fails on
// behaviour. The implementer DELETES this file and implements consumer.go and
// identity.go (mirror team-search/internal/consumer/consumer.go).

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/buidangphuc/team-domain/internal/repository"
)

var ErrPermanent = errors.New("permanent consumer error")

type Handler func(ctx context.Context, key, value []byte) error

type DLQProducer interface {
	Produce(ctx context.Context, key, value []byte) error
	Close()
}

type RetryConfig struct {
	MaxAttempts int
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
}

type Consumer struct {
	dlq DLQProducer
	cfg RetryConfig
}

// processRecord stub: single attempt, no DLQ.
func (c *Consumer) processRecord(ctx context.Context, key, value []byte, h Handler, _ *slog.Logger) error {
	return h(ctx, key, value)
}

type SellerStore interface {
	HoldSellerListings(ctx context.Context, userID, eventID string, changedAt time.Time, enqueue repository.EnqueueFn) (int, error)
	RestoreSellerListings(ctx context.Context, userID, eventID string, changedAt time.Time, enqueue repository.EnqueueFn) (int, error)
}

type IdentityConsumer struct{}

func NewIdentityConsumer(SellerStore, *slog.Logger) *IdentityConsumer { return &IdentityConsumer{} }

func (*IdentityConsumer) Handle(context.Context, []byte, []byte) error {
	return errors.New("not implemented")
}
