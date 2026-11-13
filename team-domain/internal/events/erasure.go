package events

// STUB (test-author): replaced by the implementer in T2 (tasks 1.4/1.5).

import (
	"context"
	"log/slog"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/team-domain/internal/store"
)

type ErasureStore interface {
	EraseSellerListings(ctx context.Context, userID, eventID string, enqueue store.EnqueueFn) (int, error)
}

type ErasureConsumer struct {
	store  ErasureStore
	logger *slog.Logger
}

func NewErasureConsumer(st ErasureStore, logger *slog.Logger) *ErasureConsumer {
	return &ErasureConsumer{store: st, logger: logger}
}

func (c *ErasureConsumer) Handle(ctx context.Context, rec consume.Record) error { return nil }
