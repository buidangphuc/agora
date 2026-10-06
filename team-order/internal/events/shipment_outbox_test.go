package events

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/repository"
)

func newShipmentRepo(outbox repository.OutboxRepository) *repository.InMemoryShipmentRepository {
	return repository.NewInMemoryShipmentRepository(
		repository.WithShipmentOutbox(BuildShippedOutboxRow),
		repository.WithInMemoryShipmentOutbox(outbox),
	)
}

// CreateShipment writes an OrderShipped outbox row, keyed by order id, carrying
// the buyer, seller, carrier and tracking code.
func TestCreateShipmentWritesOrderShippedRow(t *testing.T) {
	ctx := context.Background()
	outbox := repository.NewInMemoryOutboxRepository()
	repo := newShipmentRepo(outbox)

	created, err := repo.CreateShipment(ctx, repository.Shipment{
		OrderID: "ord-1", Carrier: "GHN", TrackingCode: "GHN-123",
		BuyerID: "buyer-1", SellerID: "seller-1",
	})
	require.NoError(t, err)

	rows := outbox.EnqueuedRows()
	require.Len(t, rows, 1)
	assert.Equal(t, "ord-1", rows[0].AggregateID)
	assert.Equal(t, OrderShippedEventType, rows[0].EventType)
	assert.Equal(t, OrderShippedEventID(created.ID), rows[0].EventID)

	var env eventsv1.EventEnvelope
	require.NoError(t, proto.Unmarshal(rows[0].Payload, &env))
	assert.Equal(t, rows[0].EventID, env.GetEventId())
	assert.Equal(t, OrderShippedEventType, env.GetType())
	var ev orderv1.OrderShipped
	require.NoError(t, proto.Unmarshal(env.GetPayload(), &ev))
	assert.Equal(t, "ord-1", ev.GetOrderId())
	assert.Equal(t, "buyer-1", ev.GetBuyerId())
	assert.Equal(t, "seller-1", ev.GetSellerId())
	assert.Equal(t, "GHN", ev.GetCarrier())
	assert.Equal(t, "GHN-123", ev.GetTrackingCode())
	assert.NotNil(t, ev.GetShippedAt())
}

// If the outbox row cannot be written, the shipment is not stored either.
func TestCreateShipmentRollsBackWhenOutboxFails(t *testing.T) {
	ctx := context.Background()
	repo := newShipmentRepo(failingOutbox{repository.NewInMemoryOutboxRepository()})

	_, err := repo.CreateShipment(ctx, repository.Shipment{OrderID: "ord-2", TrackingCode: "T-2", BuyerID: "b"})
	require.Error(t, err)
	_, err = repo.GetShipmentByOrderID(ctx, "ord-2")
	assert.ErrorIs(t, err, repository.ErrShipmentNotFound)
}
