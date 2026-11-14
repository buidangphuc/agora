package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"

	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-analytics/generated/platform/identity/v1"
	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

// UserErasedEventType is the EventEnvelope.Type of the erasure event.
const UserErasedEventType = "platform.identity.v1.UserErased"

// ErasureOptions is the identity.events consumer wiring.
func ErasureOptions(s config.Settings) RecordSpec {
	return RecordSpec{
		Topics: []string{s.Erasure.EventsTopic},
		Group:  s.Erasure.ConsumerGroup,
		Options: consume.Options{
			DLQTopic:       s.Erasure.DLQTopic,
			MaxPollRecords: engagementMaxPollRecords,
		},
	}
}

// NewErasureHandler purges the user of a UserErased record; other identity
// event types are skipped and poison records are dead-lettered.
func NewErasureHandler(eraser warehouse.UserEraser, fp *kafkax.Fingerprinter, logger *slog.Logger) consume.Handler {
	return func(ctx context.Context, rec consume.Record) error {
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(rec.Value, &env); err != nil {
			logger.WarnContext(ctx, "erasure record dead-lettered", "key_fp", fp.Of(rec.Key), "err", err)
			return consume.Permanent(fmt.Errorf("%w: unmarshal envelope: %w", ErrDeadLetter, err))
		}
		if env.GetType() != UserErasedEventType {
			logger.DebugContext(ctx, "identity event skipped", "key_fp", fp.Of(rec.Key), "type", env.GetType())
			return nil
		}
		var ev identityv1.UserErased
		if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
			logger.WarnContext(ctx, "erasure record dead-lettered", "key_fp", fp.Of(rec.Key), "err", err)
			return consume.Permanent(fmt.Errorf("%w: unmarshal UserErased: %w", ErrDeadLetter, err))
		}
		if ev.GetUserId() == "" {
			err := errors.New("UserErased has no user_id")
			logger.WarnContext(ctx, "erasure record dead-lettered", "key_fp", fp.Of(rec.Key), "err", err)
			return consume.Permanent(fmt.Errorf("%w: %w", ErrDeadLetter, err))
		}
		counts, err := eraser.EraseUser(ctx, ev.GetUserId())
		if err != nil {
			logger.ErrorContext(ctx, "user erasure failed", "key_fp", fp.Of(rec.Key), "err", err)
			return fmt.Errorf("erase user: %w", err)
		}
		tables := make([]string, 0, len(counts))
		for t := range counts {
			tables = append(tables, t)
		}
		sort.Strings(tables)
		attrs := []any{"key_fp", fp.Of(rec.Key)}
		for _, t := range tables {
			attrs = append(attrs, t, counts[t])
		}
		logger.InfoContext(ctx, "user erased", attrs...)
		return nil
	}
}
