package events

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"

	eventsv1 "github.com/buidangphuc/team-engagement/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-engagement/generated/platform/listing/v1"
)

// Discriminator types carried in EventEnvelope.Type.
const (
	ListingChangedType       = "platform.listing.v1.ListingChanged"
	ListingStatusChangedType = "platform.listing.v1.ListingStatusChanged"
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
// retry/DLQ discipline in consume.Consumer.Run applies.
func ListingEventHandler(store FeedStore) consume.Handler {
	return func(ctx context.Context, rec consume.Record) error {
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(rec.Value, &env); err != nil {
			return fmt.Errorf("unmarshal envelope: %w", err)
		}

		switch env.GetType() {
		case ListingChangedType:
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

		case ListingStatusChangedType:
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

// NewListingClient joins group on topic with auto-commit off. A fresh group starts
// from the earliest offset so the feed is backfilled from the whole topic history.
// opts are applied last.
func NewListingClient(k config.Kafka, group, topic string, opts ...kgo.Opt) (*kgo.Client, error) {
	return kafkax.NewConsumerClient(k, group, []string{topic},
		append([]kgo.Opt{kgo.ConsumeResetOffset(kgo.NewOffset().AtStart())}, opts...)...)
}

// NewListingConsumer returns the follow-feed consumer: ListingEventHandler over
// client, with poison records parked on <topic>.dlq. A record that can be neither
// handled nor parked stops Run with an error before its offset is committed, so a
// restart in the same group delivers it again.
func NewListingConsumer(client *kgo.Client, feed FeedStore, dlq consume.DLQProducer, topic string, logger *slog.Logger) *consume.Consumer {
	return consume.NewConsumer(client, ListingEventHandler(feed), dlq, consume.Options{
		DLQTopic: topic + ".dlq",
		Logger:   logger,
	})
}
