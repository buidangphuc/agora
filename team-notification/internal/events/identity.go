package events

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	eventsv1 "github.com/buidangphuc/team-notification/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-notification/generated/platform/identity/v1"
)

const userErasedType = "platform.identity.v1.UserErased"

type UserEraser interface {
	Erase(ctx context.Context, userID string) (int64, error)
}

func IdentityEventHandler(store UserEraser, fp *kafkax.Fingerprinter, logger *slog.Logger) consume.Handler {
	return func(ctx context.Context, r consume.Record) error {
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(r.Value, &env); err != nil {
			return consume.Permanent(fmt.Errorf("unmarshal envelope: %w", err))
		}
		if env.GetType() != userErasedType {
			return nil
		}
		var erased identityv1.UserErased
		if err := proto.Unmarshal(env.GetPayload(), &erased); err != nil {
			return consume.Permanent(fmt.Errorf("unmarshal UserErased: %w", err))
		}
		id := erased.GetUserId()
		if id == "" {
			return consume.Permanent(fmt.Errorf("UserErased has no user id"))
		}
		rows, err := store.Erase(ctx, id)
		if err != nil {
			return fmt.Errorf("erase user: %w", err)
		}
		attrs := []any{"rows", rows}
		if fp != nil {
			attrs = append(attrs, "user_fp", fp.Of([]byte(id)))
		}
		logger.InfoContext(ctx, "user erased", attrs...)
		return nil
	}
}
