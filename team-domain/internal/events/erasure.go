package events

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"

	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	eventsv1 "github.com/buidangphuc/team-domain/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-domain/generated/platform/identity/v1"
	"github.com/buidangphuc/team-domain/internal/store"
)

const userErasedType = "platform.identity.v1.UserErased"

// ErasureStore deletes every listing of a seller in one transaction,
// enqueueing one ListingChanged(DELETED) outbox row per deleted listing.
type ErasureStore interface {
	EraseSellerListings(ctx context.Context, userID, eventID string, enqueue store.EnqueueFn) (int, error)
}

// ErasureConsumer applies identity UserErased events to seller listings.
type ErasureConsumer struct {
	store  ErasureStore
	logger *slog.Logger
}

// NewErasureConsumer builds the handler over st.
func NewErasureConsumer(st ErasureStore, logger *slog.Logger) *ErasureConsumer {
	return &ErasureConsumer{store: st, logger: logger}
}

// Handle is a consume.Handler. Malformed events return an error wrapped by
// consume.Permanent; store failures are returned as-is so they are retried.
func (c *ErasureConsumer) Handle(ctx context.Context, rec consume.Record) error {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(rec.Value, &env); err != nil {
		return consume.Permanent(fmt.Errorf("unmarshal envelope: %w", err))
	}
	if env.GetType() != userErasedType {
		return nil
	}
	var ev identityv1.UserErased
	if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
		return consume.Permanent(fmt.Errorf("unmarshal UserErased: %w", err))
	}
	if ev.GetUserId() == "" {
		return consume.Permanent(errors.New("empty user_id"))
	}
	if ev.GetErasedAt() == nil {
		return consume.Permanent(errors.New("missing erased_at"))
	}
	n, err := c.store.EraseSellerListings(ctx, ev.GetUserId(), env.GetEventId(), EnqueueDeleted())
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(ev.GetUserId()))
	c.logger.InfoContext(ctx, "seller listings erased", "user_fp", hex.EncodeToString(sum[:])[:12], "listings", n)
	return nil
}
