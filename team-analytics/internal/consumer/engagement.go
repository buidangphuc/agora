package consumer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	engagementv1 "github.com/buidangphuc/team-analytics/generated/platform/engagement/v1"
	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
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
func EngagementFactFromEnvelope(value []byte) (*warehouse.EngagementFactRecord, error) {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}
	rec := &warehouse.EngagementFactRecord{EventID: env.GetEventId(), OccurredAt: time.Now().UTC()}
	if env.GetOccurredAt() != nil {
		rec.OccurredAt = env.GetOccurredAt().AsTime().UTC()
	}
	var payload proto.Message
	var fill func()
	switch env.GetType() {
	case FavoriteAddedEventType:
		m := &engagementv1.FavoriteAdded{}
		payload, rec.Fact = m, warehouse.FactFavoriteAdded
		fill = func() { rec.UserID, rec.ListingID = m.GetUserId(), m.GetListingId() }
	case FavoriteRemovedEventType:
		m := &engagementv1.FavoriteRemoved{}
		payload, rec.Fact = m, warehouse.FactFavoriteRemoved
		fill = func() { rec.UserID, rec.ListingID = m.GetUserId(), m.GetListingId() }
	case SellerFollowedEventType:
		m := &engagementv1.SellerFollowed{}
		payload, rec.Fact = m, warehouse.FactSellerFollowed
		fill = func() { rec.UserID, rec.SellerID = m.GetUserId(), m.GetSellerId() }
	case SellerUnfollowedEventType:
		m := &engagementv1.SellerUnfollowed{}
		payload, rec.Fact = m, warehouse.FactSellerUnfollowed
		fill = func() { rec.UserID, rec.SellerID = m.GetUserId(), m.GetSellerId() }
	case ReviewCreatedEventType:
		m := &engagementv1.ReviewCreated{}
		payload, rec.Fact = m, warehouse.FactReviewCreated
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
	DeadLetter(ctx context.Context, key, value []byte, reason error) error
}

// ProcessEngagementRecords maps the polled values, writes the good rows in one
// idempotent batch and dead-letters the rest. It returns nil only when every
// row is written and every bad record is dead-lettered, so the caller may then
// commit offsets. The DLQ is published first: a failed write is retried over the
// same records, and a duplicate DLQ copy is preferable to a lost one.
func ProcessEngagementRecords(ctx context.Context, recs []*kgo.Record, w warehouse.EngagementFactWriter, dlq DeadLetterer, logger *slog.Logger) error {
	var rows []*warehouse.EngagementFactRecord
	for _, r := range recs {
		row, err := EngagementFactFromEnvelope(r.Value)
		if err != nil {
			logger.Warn("engagement record dead-lettered", slog.String("key", string(r.Key)), slog.Any("err", err))
			if derr := dlq.DeadLetter(ctx, r.Key, r.Value, err); derr != nil {
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
}

// NewEngagementConsumer joins group on topic. Auto-commit is disabled; offsets
// advance only after the batch is written and every bad record is dead-lettered.
func NewEngagementConsumer(brokers []string, group, topic, dlqTopic string) (*EngagementConsumer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return nil, err
	}
	return &EngagementConsumer{client: client, dlqTopic: dlqTopic}, nil
}

// DeadLetter republishes the original record to the DLQ topic with the reason in a header.
func (c *EngagementConsumer) DeadLetter(ctx context.Context, key, value []byte, reason error) error {
	rec := &kgo.Record{
		Topic: c.dlqTopic, Key: key, Value: value,
		Headers: []kgo.RecordHeader{{Key: "error", Value: []byte(reason.Error())}},
	}
	return c.client.ProduceSync(ctx, rec).FirstErr()
}

// Run processes each polled batch, retrying it until it lands (the write is
// idempotent), then commits offsets.
func (c *EngagementConsumer) Run(ctx context.Context, w warehouse.EngagementFactWriter, logger *slog.Logger) error {
	for ctx.Err() == nil {
		fetches := c.client.PollFetches(ctx)
		if errs := fetches.Errors(); len(errs) > 0 {
			if ctx.Err() != nil {
				break
			}
			logger.Warn("engagement kafka fetch error", slog.Any("errs", errs))
			continue
		}
		var recs []*kgo.Record
		iter := fetches.RecordIter()
		for !iter.Done() {
			recs = append(recs, iter.Next())
		}
		if len(recs) == 0 {
			continue
		}
		for ctx.Err() == nil {
			err := ProcessEngagementRecords(ctx, recs, w, c, logger)
			if err == nil {
				break
			}
			logger.Warn("engagement batch failed; will retry", slog.Any("err", err))
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
		}
		if ctx.Err() != nil {
			break
		}
		if err := c.client.CommitUncommittedOffsets(ctx); err != nil && ctx.Err() == nil {
			logger.Warn("engagement offset commit failed", slog.Any("err", err))
		}
	}
	return nil
}

// Close shuts the client down.
func (c *EngagementConsumer) Close() { c.client.Close() }
