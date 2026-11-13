package events

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/team-verification/internal/objects"
)

type UserErasedConsumer struct{}

func NewUserErasedConsumer(_ *pgxpool.Pool, _ objects.Store, _ *slog.Logger) *UserErasedConsumer {
	return &UserErasedConsumer{}
}

func (c *UserErasedConsumer) Handle(_ context.Context, _ consume.Record) error { return nil }
