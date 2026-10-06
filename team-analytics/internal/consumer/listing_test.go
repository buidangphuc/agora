package consumer_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-analytics/generated/platform/listing/v1"
	"github.com/buidangphuc/team-analytics/internal/consumer"
)

func listingEnvelope(t *testing.T, typ string, lc *listingv1.ListingChanged, at time.Time) []byte {
	t.Helper()
	payload, err := proto.Marshal(lc)
	require.NoError(t, err)
	value, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: "evt-1", Type: typ, OccurredAt: timestamppb.New(at), Payload: payload})
	require.NoError(t, err)
	return value
}

func TestListingSellerFromEnvelope(t *testing.T) {
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

	// DELETED keeps the mapping so history stays attributable.
	for _, ct := range []listingv1.ChangeType{listingv1.ChangeType_CHANGE_TYPE_CREATED, listingv1.ChangeType_CHANGE_TYPE_DELETED} {
		v := listingEnvelope(t, consumer.ListingChangedEventType,
			&listingv1.ListingChanged{Listing: &listingv1.Listing{Id: "lst-1", SellerId: "seller-1"}, ChangeType: ct}, at)
		rec, ok, err := consumer.ListingSellerFromEnvelope(v)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "lst-1", rec.ListingID)
		assert.Equal(t, "seller-1", rec.SellerID)
		assert.True(t, rec.UpdatedAt.Equal(at))
	}
}

func TestListingSellerFromEnvelope_Skips(t *testing.T) {
	at := time.Now()
	_, ok, err := consumer.ListingSellerFromEnvelope(listingEnvelope(t, "platform.other.v1.Thing", &listingv1.ListingChanged{}, at))
	require.NoError(t, err)
	assert.False(t, ok, "other envelope type")

	_, ok, err = consumer.ListingSellerFromEnvelope(listingEnvelope(t, consumer.ListingChangedEventType,
		&listingv1.ListingChanged{Listing: &listingv1.Listing{Id: "lst-1"}}, at))
	require.NoError(t, err)
	assert.False(t, ok, "no seller id")

	_, _, err = consumer.ListingSellerFromEnvelope([]byte{0xff, 0xff})
	assert.Error(t, err, "malformed envelope")
}
