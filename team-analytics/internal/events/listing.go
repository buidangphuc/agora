package events

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"

	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-analytics/generated/platform/listing/v1"
	"github.com/buidangphuc/team-analytics/internal/store"
)

// ListingChangedEventType is the EventEnvelope.Type discriminator of the
// team-domain listing event that carries the listing's owner (seller_id).
const ListingChangedEventType = "platform.listing.v1.ListingChanged"

// ListingSellerFromEnvelope decodes one listing.events record into a
// listing -> seller mapping plus the listing's category and price. A well-formed envelope of another type, or a
// snapshot without a listing id or seller id, is a clean skip (ok=false). The
// change type is deliberately ignored: a DELETED event keeps the mapping so
// historical tracking stays attributable to the seller.
func ListingSellerFromEnvelope(value []byte) (rec *store.ListingSellerRecord, ok bool, err error) {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		return nil, false, fmt.Errorf("unmarshal envelope: %w", err)
	}
	if env.GetType() != ListingChangedEventType {
		return nil, false, nil
	}
	var lc listingv1.ListingChanged
	if err := proto.Unmarshal(env.GetPayload(), &lc); err != nil {
		return nil, false, fmt.Errorf("unmarshal ListingChanged: %w", err)
	}
	l := lc.GetListing()
	if l.GetId() == "" || l.GetSellerId() == "" {
		return nil, false, nil
	}
	updatedAt := time.Now().UTC()
	if env.GetOccurredAt() != nil {
		updatedAt = env.GetOccurredAt().AsTime().UTC()
	}
	return &store.ListingSellerRecord{
		ListingID: l.GetId(), SellerID: l.GetSellerId(), CategoryID: l.GetCategoryId(), Price: l.GetPrice(), UpdatedAt: updatedAt,
	}, true, nil
}

// ListingConsumer keeps the listing_sellers table current from listing.events.
// It uses its OWN consumer group, reset to the earliest offset, so listings that
// existed before this consumer was deployed are backfilled.
type ListingConsumer struct {
	client   *kgo.Client
	dlqTopic string
}

// NewListingConsumer wraps a consumer-group client with auto-commit disabled
// (kafkax.NewConsumerClient). The caller builds it with
// kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()) so listings that existed
// before the group was deployed are backfilled. A batch the warehouse still
// cannot write after the SDK batch consumer's attempts goes to dlqTopic.
func NewListingConsumer(client *kgo.Client, dlqTopic string) *ListingConsumer {
	return &ListingConsumer{client: client, dlqTopic: dlqTopic}
}

// Run upserts each polled batch through the SDK batch consumer: offsets are
// committed only after the upsert succeeded (at-least-once; the upsert is
// idempotent), a failed upsert retries the same batch with backoff, and a batch
// that still fails is dead-lettered whole. An undecodable record is skipped. It
// returns nil when ctx ends.
func (c *ListingConsumer) Run(ctx context.Context, w store.ListingSellerWriter, logger *slog.Logger) error {
	bc, err := consume.NewBatchConsumer(c.client, func(ctx context.Context, recs []consume.Record) error {
		var batch []*store.ListingSellerRecord
		for _, rec := range recs {
			m, ok, err := ListingSellerFromEnvelope(rec.Value)
			if err != nil {
				logger.WarnContext(ctx, "decode listing record failed; skipping", "key", string(rec.Key), "err", err)
				continue
			}
			if ok {
				batch = append(batch, m)
			}
		}
		if len(batch) == 0 {
			return nil
		}
		return w.UpsertListingSellers(ctx, batch)
	}, clientDLQ{c.client}, consume.BatchOptions{DLQTopic: c.dlqTopic, Logger: logger})
	if err != nil {
		return err
	}
	return bc.Run(ctx)
}

// Close shuts the client down.
func (c *ListingConsumer) Close() { c.client.Close() }
