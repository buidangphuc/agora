package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"

	engagementv1 "github.com/buidangphuc/team-analytics/generated/platform/engagement/v1"
	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	"github.com/buidangphuc/team-analytics/internal/store"
)

// Engagement EventEnvelope.Type discriminators (the payload's full name).
const (
	FavoriteAddedEventType    = "platform.engagement.v1.FavoriteAdded"
	FavoriteRemovedEventType  = "platform.engagement.v1.FavoriteRemoved"
	SellerFollowedEventType   = "platform.engagement.v1.SellerFollowed"
	SellerUnfollowedEventType = "platform.engagement.v1.SellerUnfollowed"
	ReviewCreatedEventType    = "platform.engagement.v1.ReviewCreated"
)

// ErrUnknownEngagementType marks a well-formed envelope whose type is not an
// engagement fact; it is dead-lettered like an undecodable record.
var ErrUnknownEngagementType = errors.New("unknown engagement envelope type")

// EngagementFactFromEnvelope maps one engagement.events record to an
// engagement_facts row. Any error (undecodable envelope or payload, unknown
// type) means the record belongs on the DLQ.
func EngagementFactFromEnvelope(value []byte) (*store.EngagementFactRecord, error) {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}
	rec := &store.EngagementFactRecord{EventID: env.GetEventId(), OccurredAt: time.Now().UTC()}
	if env.GetOccurredAt() != nil {
		rec.OccurredAt = env.GetOccurredAt().AsTime().UTC()
	}
	var payload proto.Message
	var fill func()
	switch env.GetType() {
	case FavoriteAddedEventType:
		m := &engagementv1.FavoriteAdded{}
		payload, rec.Fact = m, store.FactFavoriteAdded
		fill = func() { rec.UserID, rec.ListingID = m.GetUserId(), m.GetListingId() }
	case FavoriteRemovedEventType:
		m := &engagementv1.FavoriteRemoved{}
		payload, rec.Fact = m, store.FactFavoriteRemoved
		fill = func() { rec.UserID, rec.ListingID = m.GetUserId(), m.GetListingId() }
	case SellerFollowedEventType:
		m := &engagementv1.SellerFollowed{}
		payload, rec.Fact = m, store.FactSellerFollowed
		fill = func() { rec.UserID, rec.SellerID = m.GetUserId(), m.GetSellerId() }
	case SellerUnfollowedEventType:
		m := &engagementv1.SellerUnfollowed{}
		payload, rec.Fact = m, store.FactSellerUnfollowed
		fill = func() { rec.UserID, rec.SellerID = m.GetUserId(), m.GetSellerId() }
	case ReviewCreatedEventType:
		m := &engagementv1.ReviewCreated{}
		payload, rec.Fact = m, store.FactReviewCreated
		fill = func() {
			rec.UserID, rec.ListingID, rec.SellerID, rec.Rating = m.GetUserId(), m.GetListingId(), m.GetSellerId(), m.GetRating()
		}
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownEngagementType, env.GetType())
	}
	if err := proto.Unmarshal(env.GetPayload(), payload); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", env.GetType(), err)
	}
	fill()
	if rec.EventID == "" {
		return nil, errors.New("envelope has no event_id")
	}
	return rec, nil
}

// DeadLetterer republishes a record that could not be mapped.
type DeadLetterer interface {
	DeadLetter(ctx context.Context, rec consume.Record, reason error) error
}

// ProcessEngagementRecords maps the polled values, writes the good rows in one
// idempotent batch and dead-letters the rest. It returns nil only when every
// row is written and every bad record is dead-lettered, so the caller may then
// commit offsets. The DLQ is published first: a failed write is retried over the
// same records, and a duplicate DLQ copy is preferable to a lost one.
func ProcessEngagementRecords(ctx context.Context, recs []consume.Record, w store.EngagementFactWriter, dlq DeadLetterer, logger *slog.Logger) error {
	var rows []*store.EngagementFactRecord
	for _, r := range recs {
		row, err := EngagementFactFromEnvelope(r.Value)
		if err != nil {
			logger.WarnContext(ctx, "engagement record dead-lettered", "key", string(r.Key), "err", err)
			if derr := dlq.DeadLetter(ctx, r, err); derr != nil {
				return fmt.Errorf("dead-letter engagement record: %w", derr)
			}
			continue
		}
		rows = append(rows, row)
	}
	if err := w.WriteEngagementFacts(ctx, rows); err != nil {
		return fmt.Errorf("write engagement facts: %w", err)
	}
	return nil
}

// EngagementConsumer reads engagement.events in its own consumer group, writes
// engagement_facts and dead-letters undecodable records.
type EngagementConsumer struct {
	client   *kgo.Client
	dlqTopic string
	group    string
}

// NewEngagementConsumer wraps a consumer-group client with auto-commit disabled
// (kafkax.NewConsumerClient); offsets advance only after the batch is written
// and every bad record is dead-lettered.
func NewEngagementConsumer(client *kgo.Client, dlqTopic string) *EngagementConsumer {
	return &EngagementConsumer{client: client, dlqTopic: dlqTopic, group: groupOf(client)}
}

// DeadLetter republishes the original record to the DLQ topic with the kafkax dlq-* headers.
func (c *EngagementConsumer) DeadLetter(ctx context.Context, rec consume.Record, reason error) error {
	return clientDLQ{c.client}.PublishRecord(ctx, DeadLetterRecord(c.dlqTopic, c.group, rec, reason))
}

// Run hands each polled batch to ProcessEngagementRecords through the SDK batch
// consumer: the same batch is retried with backoff (the write is idempotent),
// offsets are committed once it lands, and a batch that still fails after the
// consumer's attempts is dead-lettered whole. It returns nil when ctx ends.
func (c *EngagementConsumer) Run(ctx context.Context, w store.EngagementFactWriter, logger *slog.Logger) error {
	bc, err := consume.NewBatchConsumer(c.client, func(ctx context.Context, recs []consume.Record) error {
		if err := ProcessEngagementRecords(ctx, recs, w, c, logger); err != nil {
			return fmt.Errorf("engagement batch failed: %w", err)
		}
		return nil
	}, clientDLQ{c.client}, consume.BatchOptions{DLQTopic: c.dlqTopic, Logger: logger})
	if err != nil {
		return err
	}
	return bc.Run(ctx)
}

// Close shuts the client down.
func (c *EngagementConsumer) Close() { c.client.Close() }
