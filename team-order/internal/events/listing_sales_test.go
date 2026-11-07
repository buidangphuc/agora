package events_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/store"
)

func sampleSalesChange() store.ListingSalesChange {
	return store.ListingSalesChange{
		ListingID:     "lst-sales-1",
		SoldCount:     5,
		ChangedAt:     time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC),
		SourceEventID: "src-event-order-123",
	}
}

func decodeListingSales(t *testing.T, value []byte) (*eventsv1.EventEnvelope, *orderv1.ListingSalesChanged) {
	t.Helper()
	var env eventsv1.EventEnvelope
	require.NoError(t, proto.Unmarshal(value, &env))
	var ev orderv1.ListingSalesChanged
	require.NoError(t, proto.Unmarshal(env.GetPayload(), &ev))
	return &env, &ev
}

// Scenario: The payload carries no personal identifier
func TestListingSalesChanged_PayloadHoldsOnlyListingCountAndTime(t *testing.T) {
	change := sampleSalesChange()
	row, err := events.BuildListingSalesChangedOutboxRow(change)
	require.NoError(t, err)

	assert.Equal(t, events.ListingSalesChangedEventType, row.EventType)
	assert.Equal(t, "platform.order.v1.ListingSalesChanged", row.EventType)
	assert.Equal(t, change.ListingID, row.AggregateID, "the Kafka key is the listing id")

	env, ev := decodeListingSales(t, row.Payload)
	assert.Equal(t, row.EventType, env.GetType())
	assert.Equal(t, row.EventID, env.GetEventId())
	assert.Equal(t, change.ListingID, ev.GetListingId())
	assert.Equal(t, change.SoldCount, ev.GetSoldCount())
	assert.True(t, change.ChangedAt.Equal(ev.GetChangedAt().AsTime()), "changed_at = %v", ev.GetChangedAt().AsTime())

	fields := ev.ProtoReflect().Descriptor().Fields()
	var names []string
	for i := 0; i < fields.Len(); i++ {
		names = append(names, string(fields.Get(i).Name()))
	}
	assert.ElementsMatch(t, []string{"listing_id", "sold_count", "changed_at"}, names)

	for _, secret := range []string{"order-123", "buyer", "seller"} {
		assert.False(t, bytes.Contains(row.Payload, []byte(secret)), "payload must not contain %q", secret)
		assert.NotContains(t, row.AggregateID, secret)
	}
}

// Scenario: A redelivered relay does not change the meaning
func TestListingSalesChanged_EventIDIsDeterministic(t *testing.T) {
	change := sampleSalesChange()
	a, err := events.BuildListingSalesChangedOutboxRow(change)
	require.NoError(t, err)
	b, err := events.BuildListingSalesChangedOutboxRow(change)
	require.NoError(t, err)
	assert.NotEmpty(t, a.EventID)
	assert.Equal(t, a.EventID, b.EventID)
	assert.Equal(t, events.ListingSalesChangedEventID(change.ListingID, change.SoldCount, change.SourceEventID), a.EventID)

	other := change
	other.SoldCount = 6
	c, err := events.BuildListingSalesChangedOutboxRow(other)
	require.NoError(t, err)
	assert.NotEqual(t, a.EventID, c.EventID, "a different value is a different event")

	otherListing := change
	otherListing.ListingID = "lst-sales-2"
	d, err := events.BuildListingSalesChangedOutboxRow(otherListing)
	require.NoError(t, err)
	assert.NotEqual(t, a.EventID, d.EventID)
}

// Scenario: A redelivered relay does not change the meaning
func TestListingSalesChanged_RelayedTwiceKeepsValueAndID(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	change := sampleSalesChange()
	row, err := events.BuildListingSalesChangedOutboxRow(change)
	require.NoError(t, err)
	enqueueRow(t, r, row)

	rec := testkit.NewRecorder()
	relayer, err := events.NewRelayer(r.ob, rec, relayerSettings(time.Second), quietLogger())
	require.NoError(t, err)

	report, err := relayer.RelayOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, report.Published)

	_, err = r.pool.Exec(ctx, `UPDATE order_outbox_events SET status = 'pending', published_at = NULL, available_at = now() WHERE event_id = $1`, row.EventID)
	require.NoError(t, err)
	report, err = relayer.RelayOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, report.Published)

	got := rec.Records()
	require.Len(t, got, 2)
	for _, rec := range got {
		assert.Equal(t, "order.events", rec.Topic)
		assert.Equal(t, change.ListingID, string(rec.Key))
	}
	envA, evA := decodeListingSales(t, got[0].Value)
	envB, evB := decodeListingSales(t, got[1].Value)
	assert.Equal(t, envA.GetEventId(), envB.GetEventId())
	assert.Equal(t, row.EventID, envA.GetEventId())
	assert.Equal(t, evA.GetSoldCount(), evB.GetSoldCount())
	assert.Equal(t, change.SoldCount, evA.GetSoldCount())
	assert.Equal(t, got[0].Value, got[1].Value)
}
