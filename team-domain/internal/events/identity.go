package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	commonv1 "github.com/buidangphuc/team-domain/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-domain/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-domain/generated/platform/identity/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/store"
)

const (
	userStatusChangedType = "platform.identity.v1.UserStatusChanged"
	servicePrincipalID    = "service-team-domain"
)

// SellerStore holds and restores a seller's listings in one transaction,
// enqueueing one ListingChanged outbox row per changed listing.
type SellerStore interface {
	HoldSellerListings(ctx context.Context, userID, eventID string, changedAt time.Time, enqueue store.EnqueueFn) (int, error)
	RestoreSellerListings(ctx context.Context, userID, eventID string, changedAt time.Time, enqueue store.EnqueueFn) (int, error)
}

// IdentityConsumer applies identity UserStatusChanged events to seller listings.
type IdentityConsumer struct {
	store  SellerStore
	logger *slog.Logger
}

// NewIdentityConsumer builds the handler over store.
func NewIdentityConsumer(store SellerStore, logger *slog.Logger) *IdentityConsumer {
	return &IdentityConsumer{store: store, logger: logger}
}

// Handle is a consume.Handler. Malformed events return an error wrapped by
// consume.Permanent; store failures are returned as-is so they are retried.
func (c *IdentityConsumer) Handle(ctx context.Context, rec consume.Record) error {
	value := rec.Value
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		return consume.Permanent(fmt.Errorf("unmarshal envelope: %v", err))
	}
	if env.GetType() != userStatusChangedType {
		return nil
	}
	var ev identityv1.UserStatusChanged
	if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
		return consume.Permanent(fmt.Errorf("unmarshal UserStatusChanged: %v", err))
	}
	if ev.GetUserId() == "" {
		return consume.Permanent(errors.New("empty user_id"))
	}
	if ev.GetChangedAt() == nil {
		return consume.Permanent(errors.New("missing changed_at"))
	}
	changedAt := ev.GetChangedAt().AsTime()
	enqueue := enqueueChanged()

	switch ev.GetStatus() {
	case identityv1.UserStatus_USER_STATUS_SUSPENDED, identityv1.UserStatus_USER_STATUS_BANNED:
		_, err := c.store.HoldSellerListings(ctx, ev.GetUserId(), env.GetEventId(), changedAt, enqueue)
		return err
	case identityv1.UserStatus_USER_STATUS_ACTIVE, identityv1.UserStatus_USER_STATUS_RESTRICTED:
		_, err := c.store.RestoreSellerListings(ctx, ev.GetUserId(), env.GetEventId(), changedAt, enqueue)
		return err
	default:
		return consume.Permanent(fmt.Errorf("unsupported status %d", ev.GetStatus()))
	}
}

// enqueueChanged builds the ListingChanged(UPDATED) outbox row for a listing
// changed by the hold/restore, attributed to the team-domain service principal.
func enqueueChanged() store.EnqueueFn {
	principal := &commonv1.Principal{Id: servicePrincipalID, Type: commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE}
	return func(l store.Listing) (store.OutboxRow, error) {
		eventID := uuid.NewString()
		payload, err := BuildListingChangedEnvelope(eventID, listingWire(l), listingv1.ChangeType_CHANGE_TYPE_UPDATED, principal, "")
		if err != nil {
			return store.OutboxRow{}, err
		}
		return store.OutboxRow{
			EventID:       eventID,
			AggregateType: "Listing",
			AggregateID:   l.ID,
			EventType:     ListingChangedEventType,
			Payload:       payload,
		}, nil
	}
}

func listingWire(l store.Listing) *listingv1.Listing {
	variants := make([]*listingv1.Variant, 0, len(l.Variants))
	for _, v := range l.Variants {
		variants = append(variants, &listingv1.Variant{
			Id: v.ID, ListingId: v.ListingID, Name: v.Name, Sku: v.SKU,
			Price: v.Price, Stock: v.Stock, ImageUrl: v.ImageURL,
		})
	}
	w := &listingv1.Listing{
		Id: l.ID, Title: l.Title, Description: l.Description, Price: l.Price,
		Currency: l.Currency, Status: statusWire(l.Status), SellerId: l.SellerID,
		ImageKeys: l.ImageKeys, CategoryId: l.CategoryID, Stock: l.Stock, Variants: variants,
	}
	if l.ModerationReasonCode != "" || !l.ModerationDecidedAt.IsZero() {
		m := &listingv1.ModerationInfo{ReasonCode: l.ModerationReasonCode}
		if !l.ModerationDecidedAt.IsZero() {
			m.DecidedAt = timestamppb.New(l.ModerationDecidedAt)
		}
		w.Moderation = m
	}
	return w
}

func statusWire(s string) listingv1.ListingStatus {
	switch s {
	case "draft":
		return listingv1.ListingStatus_LISTING_STATUS_DRAFT
	case "published":
		return listingv1.ListingStatus_LISTING_STATUS_PUBLISHED
	case "rejected":
		return listingv1.ListingStatus_LISTING_STATUS_REJECTED
	case "banned":
		return listingv1.ListingStatus_LISTING_STATUS_BANNED
	case "pending_review":
		return listingv1.ListingStatus_LISTING_STATUS_PENDING_REVIEW
	case "held":
		return listingv1.ListingStatus_LISTING_STATUS_HELD
	default:
		return listingv1.ListingStatus_LISTING_STATUS_UNSPECIFIED
	}
}
