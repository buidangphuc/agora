package events

import (
	"errors"
	"fmt"
	"time"

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
	ReviewHiddenEventType     = "platform.engagement.v1.ReviewHidden"
)

// ErrUnknownEngagementType marks a well-formed envelope whose type is not an
// engagement fact; it is dead-lettered like an undecodable record.
var (
	ErrUnknownEngagementType = errors.New("unknown engagement envelope type")
	ErrDeadLetter            = errors.New("dead letter")
)

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
			rec.ReviewID = m.GetReviewId()
		}
	case ReviewHiddenEventType:
		m := &engagementv1.ReviewHidden{}
		payload, rec.Fact = m, warehouse.FactReviewHidden
		fill = func() {
			rec.ReviewID, rec.ListingID = m.GetReviewId(), m.GetListingId()
			if m.GetHiddenAt() != nil {
				rec.OccurredAt = m.GetHiddenAt().AsTime().UTC()
			}
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
