package consumer_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	engagementv1 "github.com/buidangphuc/team-analytics/generated/platform/engagement/v1"
	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	"github.com/buidangphuc/team-analytics/internal/consumer"
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
		{"favorite added", consumer.FavoriteAddedEventType, &engagementv1.FavoriteAdded{UserId: "u", ListingId: "l"},
			warehouse.EngagementFactRecord{Fact: "favorite_added", UserID: "u", ListingID: "l"}},
		{"favorite removed", consumer.FavoriteRemovedEventType, &engagementv1.FavoriteRemoved{UserId: "u", ListingId: "l"},
			warehouse.EngagementFactRecord{Fact: "favorite_removed", UserID: "u", ListingID: "l"}},
		{"seller followed", consumer.SellerFollowedEventType, &engagementv1.SellerFollowed{UserId: "u", SellerId: "s"},
			warehouse.EngagementFactRecord{Fact: "seller_followed", UserID: "u", SellerID: "s"}},
		{"seller unfollowed", consumer.SellerUnfollowedEventType, &engagementv1.SellerUnfollowed{UserId: "u", SellerId: "s"},
			warehouse.EngagementFactRecord{Fact: "seller_unfollowed", UserID: "u", SellerID: "s"}},
		{"review created", consumer.ReviewCreatedEventType, &engagementv1.ReviewCreated{ReviewId: "r", UserId: "u", ListingId: "l", SellerId: "s", Rating: 4},
			warehouse.EngagementFactRecord{Fact: "review_created", UserID: "u", ListingID: "l", SellerID: "s", Rating: 4}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := consumer.EngagementFactFromEnvelope(engEnvelope(t, "evt-1", c.typ, c.payload))
			require.NoError(t, err)
			c.want.EventID, c.want.OccurredAt = "evt-1", engAt
			assert.Equal(t, c.want, *got)
		})
	}
}

func TestEngagementFactFromEnvelopeRejects(t *testing.T) {
	_, err := consumer.EngagementFactFromEnvelope([]byte("not a protobuf \xff\xff"))
	assert.Error(t, err)

	_, err = consumer.EngagementFactFromEnvelope(engEnvelope(t, "e", "platform.order.v1.OrderPaidEvent", &engagementv1.FavoriteAdded{}))
	assert.ErrorIs(t, err, consumer.ErrUnknownEngagementType)

	_, err = consumer.EngagementFactFromEnvelope(engEnvelope(t, "", consumer.FavoriteAddedEventType, &engagementv1.FavoriteAdded{UserId: "u"}))
	assert.Error(t, err)

	b, _ := proto.Marshal(&eventsv1.EventEnvelope{EventId: "e", Type: consumer.FavoriteAddedEventType, Payload: []byte{0xff, 0xff}})
	_, err = consumer.EngagementFactFromEnvelope(b)
	assert.Error(t, err)
}

type engWriter struct {
	rows []*warehouse.EngagementFactRecord
	fail error
}

func (w *engWriter) WriteEngagementFacts(_ context.Context, b []*warehouse.EngagementFactRecord) error {
	if w.fail != nil {
		return w.fail
	}
	w.rows = append(w.rows, b...)
	return nil
}

type engDLQ struct {
	keys [][]byte
	vals [][]byte
	fail error
}

func (d *engDLQ) DeadLetter(_ context.Context, k, v []byte, _ error) error {
	if d.fail != nil {
		return d.fail
	}
	d.keys, d.vals = append(d.keys, k), append(d.vals, v)
	return nil
}

func TestProcessEngagementRecordsDeadLettersAndContinues(t *testing.T) {
	bad := []byte("garbage \xff\xff")
	good := engEnvelope(t, "e1", consumer.FavoriteAddedEventType, &engagementv1.FavoriteAdded{UserId: "u", ListingId: "l"})
	recs := []*kgo.Record{{Key: []byte("k0"), Value: bad}, {Key: []byte("k1"), Value: good}}
	w, d := &engWriter{}, &engDLQ{}

	require.NoError(t, consumer.ProcessEngagementRecords(context.Background(), recs, w, d, slog.Default()))
	require.Len(t, w.rows, 1)
	assert.Equal(t, "e1", w.rows[0].EventID)
	require.Len(t, d.vals, 1)
	assert.Equal(t, bad, d.vals[0])
	assert.Equal(t, []byte("k0"), d.keys[0])
}

func TestProcessEngagementRecordsFailuresAreRetryable(t *testing.T) {
	good := engEnvelope(t, "e1", consumer.FavoriteAddedEventType, &engagementv1.FavoriteAdded{UserId: "u", ListingId: "l"})
	recs := []*kgo.Record{{Value: good}}
	err := consumer.ProcessEngagementRecords(context.Background(), recs, &engWriter{fail: errors.New("disk")}, &engDLQ{}, slog.Default())
	assert.Error(t, err, "a failed write must surface so offsets are not committed")

	err = consumer.ProcessEngagementRecords(context.Background(), []*kgo.Record{{Value: []byte("\xff")}}, &engWriter{}, &engDLQ{fail: errors.New("broker")}, slog.Default())
	assert.Error(t, err, "a failed DLQ publish must surface so offsets are not committed")
}
