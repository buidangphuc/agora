package events

import (
	"context"
	"log/slog"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"

	"github.com/buidangphuc/team-order/internal/config"
)

const (
	outboxPurgeEvery = time.Hour
	outboxRetention  = 7 * 24 * time.Hour
)

func NewRelayer(store *outbox.Store, publisher outbox.Publisher, s config.Settings, logger *slog.Logger) (*outbox.Relayer, error) {
	return outbox.NewRelayer(store, publisher, outbox.RelayerOptions{
		Topic:  s.Events.OrderTopic,
		Config: s.Outbox,
		Logger: logger,
	})
}

func NewOutboxPurger(store *outbox.Store, logger *slog.Logger) (lifecycle.Component, error) {
	if logger == nil {
		logger = slog.Default()
	}
	return lifecycle.Every(outboxPurgeEvery, func(ctx context.Context) error {
		n, err := store.Purge(ctx, outboxRetention)
		if err != nil {
			return err
		}
		if n > 0 {
			logger.InfoContext(ctx, "outbox purged", "rows", n)
		}
		return nil
	}, lifecycle.RunAtStart(), lifecycle.WithLogger(logger))
}
