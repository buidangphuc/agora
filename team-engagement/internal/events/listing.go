package events

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	eventsv1 "github.com/buidangphuc/team-engagement/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-engagement/generated/platform/listing/v1"
)

const (
	listingChangedType       = "platform.listing.v1.ListingChanged"
	listingStatusChangedType = "platform.listing.v1.ListingStatusChanged"
)

type FeedStore interface {
	UpsertSellerListing(ctx context.Context, sellerID, listingID string, createdAt time.Time) error
	RemoveSellerListing(ctx context.Context, listingID string) error
}

func NewListingClient(k config.Kafka, group, topic string) (*kgo.Client, error) {
	return kafkax.NewConsumerClient(k, group, []string{topic},
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.AllowAutoTopicCreation(),
	)
}

func NewListingConsumer(client *kgo.Client, store FeedStore, dlq consume.DLQProducer, topic string, fp *kafkax.Fingerprinter, logger *slog.Logger) (*consume.Consumer, error) {
	return consume.NewConsumer(client, ListingEventHandler(store), dlq, consume.Options{
		DLQTopic:    topic + ".dlq",
		MaxAttempts: 5,
		Logger:      logger,
		KeyFP:       fp,
	})
}

func ListingEventHandler(store FeedStore) consume.Handler {
	return func(ctx context.Context, r consume.Record) error {
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(r.Value, &env); err != nil {
			return consume.Permanent(fmt.Errorf("unmarshal envelope: %w", err))
		}
		switch env.GetType() {
		case listingChangedType:
			var changed listingv1.ListingChanged
			if err := proto.Unmarshal(env.GetPayload(), &changed); err != nil {
				return consume.Permanent(fmt.Errorf("unmarshal ListingChanged: %w", err))
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
				return consume.Permanent(fmt.Errorf("unmarshal ListingStatusChanged: %w", err))
			}
			if st.GetListingId() == "" {
				return fmt.Errorf("event has no listing id")
			}
			if st.GetStatus() != listingv1.ListingStatus_LISTING_STATUS_PUBLISHED {
				return store.RemoveSellerListing(ctx, st.GetListingId())
			}
			return nil
		default:
			return nil
		}
	}
}
