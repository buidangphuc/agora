package events

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-order/generated/platform/identity/v1"
)

const userErasedType = "platform.identity.v1.UserErased"

type UserEraser interface {
	EraseUser(ctx context.Context, userID string) error
}

type ErasureConsumer struct {
	store  UserEraser
	logger *slog.Logger
}

func NewErasureConsumer(store UserEraser, logger *slog.Logger) *ErasureConsumer {
	if logger == nil {
		logger = slog.Default()
	}
	return &ErasureConsumer{store: store, logger: logger}
}

func (c *ErasureConsumer) Handle(ctx context.Context, rec consume.Record) error {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(rec.Value, &env); err != nil {
		return permanent("unmarshal envelope: %v", err)
	}
	if env.GetType() != userErasedType {
		return nil
	}
	var ev identityv1.UserErased
	if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
		return permanent("unmarshal UserErased: %v", err)
	}
	if ev.GetUserId() == "" {
		return permanent("UserErased missing user_id")
	}
	if err := c.store.EraseUser(ctx, ev.GetUserId()); err != nil {
		return fmt.Errorf("erase user: %w", err)
	}
	c.logger.InfoContext(ctx, "user erasure applied", "event_id", env.GetEventId())
	return nil
}
