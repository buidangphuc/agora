package consumer

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-engagement/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-engagement/generated/platform/listing/v1"
)

// Discriminator types carried in EventEnvelope.Type.
const (
	listingChangedType       = "platform.listing.v1.ListingChanged"
	listingStatusChangedType = "platform.listing.v1.ListingStatusChanged"
)

// FeedStore is the slice of the repository the consumer writes: the follow-feed
// source (seller_listings). Both operations are idempotent, which is what makes
// at-least-once delivery safe.
type FeedStore interface {
	UpsertSellerListing(ctx context.Context, sellerID, listingID string, createdAt time.Time) error
	RemoveSellerListing(ctx context.Context, listingID string) error
}

// ListingEventHandler keeps seller_listings in step with listing.events:
//   - ListingChanged with a PUBLISHED listing (created or updated) is upserted,
//     stamped with the event's occurred_at as created_at (kept on redelivery);
//   - ListingChanged DELETED, or any non-published status, removes the row;
//   - ListingStatusChanged to a non-published status removes the row (it carries
//     no seller id, so a transition to PUBLISHED is picked up from ListingChanged).
//
// Other event types are ignored. A malformed record returns an error so the
// retry/DLQ discipline in Consumer.Run applies.
func ListingEventHandler(store FeedStore) Handler {
	return func(ctx context.Context, _ []byte, value []byte) error {
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(value, &env); err != nil {
			return fmt.Errorf("unmarshal envelope: %w", err)
		}

		switch env.GetType() {
		case listingChangedType:
			var changed listingv1.ListingChanged
			if err := proto.Unmarshal(env.GetPayload(), &changed); err != nil {
				return fmt.Errorf("unmarshal ListingChanged: %w", err)
			}
			l := changed.GetListing()
			if l == nil || l.GetId() == "" {
				return fmt.Errorf("event has no listing id")
			}
			if changed.GetChangeType() == listingv1.ChangeType_CHANGE_TYPE_DELETED ||
				l.GetStatus() != listingv1.ListingStatus_LISTING_STATUS_PUBLISHED {
				return store.RemoveSellerListing(ctx, l.GetId())
			}
			if l.GetSellerId() == "" {
				return fmt.Errorf("published listing %s has no seller id", l.GetId())
			}
			createdAt := time.Now().UTC()
			if ts := env.GetOccurredAt(); ts != nil {
				createdAt = ts.AsTime()
			}
			return store.UpsertSellerListing(ctx, l.GetSellerId(), l.GetId(), createdAt)

		case listingStatusChangedType:
			var st listingv1.ListingStatusChanged
			if err := proto.Unmarshal(env.GetPayload(), &st); err != nil {
				return fmt.Errorf("unmarshal ListingStatusChanged: %w", err)
			}
			if st.GetListingId() == "" {
				return fmt.Errorf("event has no listing id")
			}
			if st.GetStatus() != listingv1.ListingStatus_LISTING_STATUS_PUBLISHED {
				return store.RemoveSellerListing(ctx, st.GetListingId())
			}
			return nil

		default:
			return nil // not ours; ignore
		}
	}
}
