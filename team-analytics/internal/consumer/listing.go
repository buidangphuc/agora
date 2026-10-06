package consumer

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-analytics/generated/platform/listing/v1"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

// ListingChangedEventType is the EventEnvelope.Type discriminator of the
// team-domain listing event that carries the listing's owner (seller_id).
const ListingChangedEventType = "platform.listing.v1.ListingChanged"

// ListingSellerFromEnvelope decodes one listing.events record into a
// listing -> seller mapping. A well-formed envelope of another type, or a
// snapshot without a listing id or seller id, is a clean skip (ok=false). The
// change type is deliberately ignored: a DELETED event keeps the mapping so
// historical tracking stays attributable to the seller.
func ListingSellerFromEnvelope(value []byte) (rec *warehouse.ListingSellerRecord, ok bool, err error) {
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
	return &warehouse.ListingSellerRecord{ListingID: l.GetId(), SellerID: l.GetSellerId(), UpdatedAt: updatedAt}, true, nil
}

// ListingConsumer keeps the listing_sellers table current from listing.events.
// It uses its OWN consumer group, reset to the earliest offset, so listings that
// existed before this consumer was deployed are backfilled.
type ListingConsumer struct {
	client *kgo.Client
}

// NewListingConsumer joins group on topic, starting from the earliest offset
// when the group has no committed offsets. Auto-commit is disabled.
func NewListingConsumer(brokers []string, group, topic string) (*ListingConsumer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return nil, err
	}
	return &ListingConsumer{client: client}, nil
}

// Run upserts each polled batch, committing offsets only after the upsert
// succeeded (at-least-once; the upsert is idempotent). On a write failure the
// batch is retried after a short pause and offsets stay uncommitted.
func (c *ListingConsumer) Run(ctx context.Context, w warehouse.ListingSellerWriter, logger *slog.Logger) error {
	for ctx.Err() == nil {
		fetches := c.client.PollFetches(ctx)
		if errs := fetches.Errors(); len(errs) > 0 {
			if ctx.Err() != nil {
				break
			}
			logger.Warn("listing kafka fetch error", slog.Any("errs", errs))
			continue
		}
		var batch []*warehouse.ListingSellerRecord
		iter := fetches.RecordIter()
		for !iter.Done() {
			rec := iter.Next()
			m, ok, err := ListingSellerFromEnvelope(rec.Value)
			if err != nil {
				logger.Warn("decode listing record failed; skipping", slog.String("key", string(rec.Key)), slog.Any("err", err))
				continue
			}
			if ok {
				batch = append(batch, m)
			}
		}
		// Retry the same batch until it lands: already-fetched records are not
		// re-polled, and the upsert is idempotent, so retrying is safe.
		for len(batch) > 0 && ctx.Err() == nil {
			err := w.UpsertListingSellers(ctx, batch)
			if err == nil {
				break
			}
			logger.Warn("listing_sellers upsert failed; will retry", slog.Any("err", err))
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
		}
		if ctx.Err() != nil {
			break
		}
		if err := c.client.CommitUncommittedOffsets(ctx); err != nil && ctx.Err() == nil {
			logger.Warn("listing offset commit failed", slog.Any("err", err))
		}
	}
	return nil
}

// Close shuts the client down.
func (c *ListingConsumer) Close() { c.client.Close() }
