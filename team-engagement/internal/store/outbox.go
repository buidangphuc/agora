package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/team-engagement/internal/events"
)

func enqueueFact(ctx context.Context, ob *outbox.Store, tx pgx.Tx, aggregateType, key string, msg proto.Message) error {
	row, err := events.FactRow(ctx, aggregateType, key, msg)
	if err != nil {
		return err
	}
	if err := ob.EnqueueTx(ctx, tx, row); err != nil {
		return fmt.Errorf("enqueue fact: %w", err)
	}
	return nil
}
