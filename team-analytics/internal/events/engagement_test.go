package events_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	engagementv1 "github.com/buidangphuc/team-analytics/generated/platform/engagement/v1"
	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	"github.com/buidangphuc/team-analytics/internal/events"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

var engAt = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

func engEnvelope(t *testing.T, id, typ string, payload proto.Message) []byte {
	t.Helper()
	pb, err := proto.Marshal(payload)
	require.NoError(t, err)
	b, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: id, Type: typ, Payload: pb, OccurredAt: timestamppb.New(engAt)})
	require.NoError(t, err)
	return b
}

func TestEngagementFactFromEnvelope(t *testing.T) {
	cases := []struct {
		name    string
		typ     string
		payload proto.Message
		want    warehouse.EngagementFactRecord
	}{
		{
			"favorite added", events.FavoriteAddedEventType, &engagementv1.FavoriteAdded{UserId: "u", ListingId: "l"},
			warehouse.EngagementFactRecord{Fact: "favorite_added", UserID: "u", ListingID: "l"},
		},
		{
			"favorite removed", events.FavoriteRemovedEventType, &engagementv1.FavoriteRemoved{UserId: "u", ListingId: "l"},
			warehouse.EngagementFactRecord{Fact: "favorite_removed", UserID: "u", ListingID: "l"},
		},
		{
			"seller followed", events.SellerFollowedEventType, &engagementv1.SellerFollowed{UserId: "u", SellerId: "s"},
			warehouse.EngagementFactRecord{Fact: "seller_followed", UserID: "u", SellerID: "s"},
		},
		{
			"seller unfollowed", events.SellerUnfollowedEventType, &engagementv1.SellerUnfollowed{UserId: "u", SellerId: "s"},
			warehouse.EngagementFactRecord{Fact: "seller_unfollowed", UserID: "u", SellerID: "s"},
		},
		{
			"review created", events.ReviewCreatedEventType, &engagementv1.ReviewCreated{ReviewId: "r", UserId: "u", ListingId: "l", SellerId: "s", Rating: 4},
			warehouse.EngagementFactRecord{Fact: "review_created", ReviewID: "r", UserID: "u", ListingID: "l", SellerID: "s", Rating: 4},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := events.EngagementFactFromEnvelope(engEnvelope(t, "evt-1", c.typ, c.payload))
			require.NoError(t, err)
			c.want.EventID, c.want.OccurredAt = "evt-1", engAt
			assert.Equal(t, c.want, *got)
		})
	}
}

func TestEngagementFactFromEnvelopeReviewHidden(t *testing.T) {
	hiddenAt := time.Date(2026, 10, 2, 8, 30, 0, 0, time.UTC)

	t.Run("Analytics review hidden: a ReviewHidden envelope maps to a review_hidden fact", func(t *testing.T) {
		got, err := events.EngagementFactFromEnvelope(engEnvelope(t, "evt-h", events.ReviewHiddenEventType,
			&engagementv1.ReviewHidden{ReviewId: "r1", ListingId: "l1", HiddenAt: timestamppb.New(hiddenAt)}))
		require.NoError(t, err)
		assert.Equal(t, "platform.engagement.v1.ReviewHidden", events.ReviewHiddenEventType)
		assert.Equal(t, warehouse.EngagementFactRecord{
			EventID: "evt-h", Fact: "review_hidden", ReviewID: "r1", ListingID: "l1", OccurredAt: hiddenAt,
		}, *got)
		assert.Empty(t, got.UserID, "a hidden fact carries no user id")
		assert.Equal(t, "review_hidden", warehouse.FactReviewHidden)
	})

	t.Run("Analytics review hidden: a corrupt payload is dead-lettered", func(t *testing.T) {
		b, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: "bad", Type: events.ReviewHiddenEventType, Payload: []byte{0xff, 0xff}})
		require.NoError(t, err)
		_, err = events.EngagementFactFromEnvelope(b)
		require.Error(t, err)
		assert.NotErrorIs(t, err, events.ErrUnknownEngagementType, "type is known; the payload is corrupt")
	})

	t.Run("Analytics review hidden: existing engagement types are unchanged", func(t *testing.T) {
		got, err := events.EngagementFactFromEnvelope(engEnvelope(t, "evt-f", events.FavoriteAddedEventType,
			&engagementv1.FavoriteAdded{UserId: "u", ListingId: "l"}))
		require.NoError(t, err)
		assert.Equal(t, warehouse.EngagementFactRecord{EventID: "evt-f", Fact: "favorite_added", UserID: "u", ListingID: "l", OccurredAt: engAt}, *got)
		assert.Empty(t, got.ReviewID)
	})
}

func TestReviewHiddenOccurredAtFallback(t *testing.T) {
	got, err := events.EngagementFactFromEnvelope(engEnvelope(t, "evt-h2", events.ReviewHiddenEventType,
		&engagementv1.ReviewHidden{ReviewId: "r2", ListingId: "l2"}))
	require.NoError(t, err)
	assert.Equal(t, engAt, got.OccurredAt)
	assert.Equal(t, "r2", got.ReviewID)
	assert.Equal(t, "review_hidden", got.Fact)
}

func TestEngagementFactFromEnvelopeRejects(t *testing.T) {
	_, err := events.EngagementFactFromEnvelope([]byte("not a protobuf \xff\xff"))
	assert.Error(t, err)

	_, err = events.EngagementFactFromEnvelope(engEnvelope(t, "e", "platform.order.v1.OrderPaidEvent", &engagementv1.FavoriteAdded{}))
	assert.ErrorIs(t, err, events.ErrUnknownEngagementType)

	_, err = events.EngagementFactFromEnvelope(engEnvelope(t, "", events.FavoriteAddedEventType, &engagementv1.FavoriteAdded{UserId: "u"}))
	assert.Error(t, err)

	b, _ := proto.Marshal(&eventsv1.EventEnvelope{EventId: "e", Type: events.FavoriteAddedEventType, Payload: []byte{0xff, 0xff}})
	_, err = events.EngagementFactFromEnvelope(b)
	assert.Error(t, err)
}
