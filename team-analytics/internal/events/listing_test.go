package events_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-analytics/generated/platform/listing/v1"
	"github.com/buidangphuc/team-analytics/internal/events"
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
		v := listingEnvelope(t, events.ListingChangedEventType,
			&listingv1.ListingChanged{Listing: &listingv1.Listing{Id: "lst-1", SellerId: "seller-1", CategoryId: "cat-books", Price: 125000}, ChangeType: ct}, at)
		rec, ok, err := events.ListingSellerFromEnvelope(v)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "lst-1", rec.ListingID)
		assert.Equal(t, "seller-1", rec.SellerID)
		assert.Equal(t, "cat-books", rec.CategoryID)
		assert.EqualValues(t, 125000, rec.Price)
		assert.True(t, rec.UpdatedAt.Equal(at))
	}
}

func TestListingSellerFromEnvelope_Skips(t *testing.T) {
	at := time.Now()
	_, ok, err := events.ListingSellerFromEnvelope(listingEnvelope(t, "platform.other.v1.Thing", &listingv1.ListingChanged{}, at))
	require.NoError(t, err)
	assert.False(t, ok, "other envelope type")

	_, ok, err = events.ListingSellerFromEnvelope(listingEnvelope(t, events.ListingChangedEventType,
		&listingv1.ListingChanged{Listing: &listingv1.Listing{Id: "lst-1"}}, at))
	require.NoError(t, err)
	assert.False(t, ok, "no seller id")

	_, _, err = events.ListingSellerFromEnvelope([]byte{0xff, 0xff})
	assert.Error(t, err, "malformed envelope")
}

func statusEnvelope(t *testing.T, status listingv1.ListingStatus, ct listingv1.ChangeType) []byte {
	t.Helper()
	return listingEnvelope(t, events.ListingChangedEventType,
		&listingv1.ListingChanged{Listing: &listingv1.Listing{Id: "lst-1", SellerId: "seller-1", Status: status}, ChangeType: ct},
		time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC))
}

func TestListingSellerFromEnvelope_StatusIsEnumName(t *testing.T) {
	rec, ok, err := events.ListingSellerFromEnvelope(statusEnvelope(t, listingv1.ListingStatus_LISTING_STATUS_PUBLISHED, listingv1.ChangeType_CHANGE_TYPE_UPDATED))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "LISTING_STATUS_PUBLISHED", rec.Status)
}

func TestListingSellerFromEnvelope_AllStatuses(t *testing.T) {
	seen := 0
	sawDeleted := false
	for num, name := range listingv1.ListingStatus_name {
		if num == 0 {
			continue
		}
		seen++
		if name == "LISTING_STATUS_DELETED" {
			sawDeleted = true
		}
		rec, ok, err := events.ListingSellerFromEnvelope(statusEnvelope(t, listingv1.ListingStatus(num), listingv1.ChangeType_CHANGE_TYPE_UPDATED))
		require.NoError(t, err, name)
		require.True(t, ok, name)
		assert.Equal(t, name, rec.Status)
	}
	assert.Positive(t, seen)
	assert.True(t, sawDeleted, "LISTING_STATUS_DELETED must be in the generated enum")
	assert.Equal(t, "LISTING_STATUS_DELETED", listingv1.ListingStatus_LISTING_STATUS_DELETED.String())
}

func TestListingSellerFromEnvelope_DeletedIsDeletedStatus(t *testing.T) {
	rec, ok, err := events.ListingSellerFromEnvelope(statusEnvelope(t, listingv1.ListingStatus_LISTING_STATUS_PUBLISHED, listingv1.ChangeType_CHANGE_TYPE_DELETED))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "lst-1", rec.ListingID)
	assert.Equal(t, "seller-1", rec.SellerID)
	assert.Equal(t, "LISTING_STATUS_DELETED", rec.Status)
}

func TestListingSellerFromEnvelope_DeletedOverridesCarriedStatus(t *testing.T) {
	for _, carried := range []listingv1.ListingStatus{
		listingv1.ListingStatus_LISTING_STATUS_UNSPECIFIED,
		listingv1.ListingStatus_LISTING_STATUS_BANNED,
		listingv1.ListingStatus(99),
	} {
		rec, ok, err := events.ListingSellerFromEnvelope(statusEnvelope(t, carried, listingv1.ChangeType_CHANGE_TYPE_DELETED))
		require.NoError(t, err, carried.String())
		require.True(t, ok, carried.String())
		assert.Equal(t, "LISTING_STATUS_DELETED", rec.Status, "carried %d", int32(carried))
	}
}

func TestListingSellerFromEnvelope_OnlyDeletedChangeTypeIsDeleted(t *testing.T) {
	for _, ct := range []listingv1.ChangeType{listingv1.ChangeType_CHANGE_TYPE_CREATED, listingv1.ChangeType_CHANGE_TYPE_UPDATED} {
		rec, ok, err := events.ListingSellerFromEnvelope(statusEnvelope(t, listingv1.ListingStatus_LISTING_STATUS_PUBLISHED, ct))
		require.NoError(t, err, ct.String())
		require.True(t, ok, ct.String())
		assert.Equal(t, "LISTING_STATUS_PUBLISHED", rec.Status, ct.String())
		assert.NotEqual(t, "LISTING_STATUS_DELETED", rec.Status, ct.String())
	}
}

func TestListingSellerFromEnvelope_UnspecifiedIsNull(t *testing.T) {
	rec, ok, err := events.ListingSellerFromEnvelope(statusEnvelope(t, listingv1.ListingStatus_LISTING_STATUS_UNSPECIFIED, listingv1.ChangeType_CHANGE_TYPE_UPDATED))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Empty(t, rec.Status)
}

func TestListingSellerFromEnvelope_UnknownEnumIsNull(t *testing.T) {
	rec, ok, err := events.ListingSellerFromEnvelope(statusEnvelope(t, listingv1.ListingStatus(99), listingv1.ChangeType_CHANGE_TYPE_UPDATED))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Empty(t, rec.Status)
}

func TestListingSellerFromEnvelope_MalformedEvent(t *testing.T) {
	rec, ok, err := events.ListingSellerFromEnvelope([]byte{0xff, 0xff})
	require.Error(t, err)
	assert.False(t, ok)
	assert.Nil(t, rec)

	env, merr := proto.Marshal(&eventsv1.EventEnvelope{EventId: "evt-1", Type: events.ListingChangedEventType, OccurredAt: timestamppb.Now(), Payload: []byte{0xff, 0xff}})
	require.NoError(t, merr)
	rec, ok, err = events.ListingSellerFromEnvelope(env)
	require.Error(t, err)
	assert.False(t, ok)
	assert.Nil(t, rec)
}
