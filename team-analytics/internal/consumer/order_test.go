package consumer_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-analytics/generated/platform/order/v1"
	"github.com/buidangphuc/team-analytics/internal/consumer"
)

func TestOrderFactsFromEnvelope(t *testing.T) {
	at := time.Date(2026, 9, 20, 14, 0, 0, 0, time.UTC)
	ope := &orderv1.OrderPaidEvent{
		OrderId: "order-456",
		BuyerId: "buyer-7",
		Items: []*orderv1.OrderLineItemFact{
			{
				ListingId: "listing-1",
				VariantId: "var-1",
				SellerId:  "seller-1",
				Quantity:  2,
				UnitPrice: 150000,
				Currency:  "VND",
			},
			{
				ListingId: "listing-2",
				VariantId: "",
				SellerId:  "seller-1",
				Quantity:  1,
				UnitPrice: 50000,
				Currency:  "VND",
			},
		},
		TotalAmount: 350000,
		Currency:    "VND",
		PaidAt:      timestamppb.New(at),
	}

	payload, err := proto.Marshal(ope)
	require.NoError(t, err)

	env := &eventsv1.EventEnvelope{
		EventId:    "evt-order-1",
		Type:       consumer.OrderPaidEventType,
		OccurredAt: timestamppb.New(at),
		Payload:    payload,
	}
	value, err := proto.Marshal(env)
	require.NoError(t, err)

	records, ok, err := consumer.OrderFactsFromEnvelope(value)
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, records, 2)

	assert.Equal(t, "evt-order-1-0", records[0].EventID)
	assert.Equal(t, "order-456", records[0].OrderID)
	assert.Equal(t, "listing-1", records[0].ListingID)
	assert.Equal(t, "var-1", records[0].VariantID)
	assert.Equal(t, "seller-1", records[0].SellerID)
	assert.Equal(t, int32(2), records[0].Quantity)
	assert.Equal(t, int64(150000), records[0].UnitPrice)
	assert.Equal(t, "VND", records[0].Currency)
	assert.Equal(t, "PAID", records[0].Status)

	assert.Equal(t, "evt-order-1-1", records[1].EventID)
	assert.Equal(t, "order-456", records[1].OrderID)
	assert.Equal(t, "listing-2", records[1].ListingID)
	assert.Equal(t, int32(1), records[1].Quantity)
	assert.Equal(t, int64(50000), records[1].UnitPrice)
}

func TestOrderFactsFromNonOrderEnvelope(t *testing.T) {
	env := &eventsv1.EventEnvelope{
		EventId: "evt-random",
		Type:    "other.event.Type",
	}
	value, err := proto.Marshal(env)
	require.NoError(t, err)

	records, ok, err := consumer.OrderFactsFromEnvelope(value)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, records)
}
