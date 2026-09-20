package events

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/buidangphuc/team-order/internal/repository"
)

type mockPublisher struct {
	mu        sync.Mutex
	published []struct {
		topic   string
		key     string
		payload []byte
	}
}

func (m *mockPublisher) Publish(ctx context.Context, topic, key string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published = append(m.published, struct {
		topic   string
		key     string
		payload []byte
	}{topic: topic, key: key, payload: payload})
	return nil
}

func TestOrderOutboxAndRelayer(t *testing.T) {
	ctx := context.Background()
	outboxRepo := repository.NewInMemoryOutboxRepository()
	pub := &mockPublisher{}
	relayer := NewRelayer(outboxRepo, pub, RelayerConfig{BatchSize: 10}, slog.Default())

	orderRepo := repository.NewInMemoryOrderRepository()
	orderRepo.SetOnPaidHook(func(ctx context.Context, o repository.Order) error {
		eventID := uuid.NewString()
		payload, err := BuildOrderPaidEnvelope(eventID, o, time.Now(), "")
		if err != nil {
			return err
		}
		return outboxRepo.Enqueue(ctx, repository.OutboxRow{
			EventID:       eventID,
			AggregateType: "Order",
			AggregateID:   o.ID,
			EventType:     OrderPaidEventType,
			Payload:       payload,
		})
	})

	// 1. Create order
	created, err := orderRepo.CreateOrder(ctx, repository.Order{
		ID:          "order-123",
		BuyerID:     "buyer-1",
		SellerID:    "seller-1",
		Status:      repository.OrderStatusPending,
		TotalAmount: 250000,
		Currency:    "VND",
		Items: []repository.OrderItem{
			{
				ID:        "item-1",
				ListingID: "listing-1",
				Quantity:  2,
				UnitPrice: 100000,
			},
			{
				ID:        "item-2",
				ListingID: "listing-2",
				Quantity:  1,
				UnitPrice: 50000,
			},
		},
	})
	require.NoError(t, err)

	// In PENDING status, no outbox event
	assert.Empty(t, outboxRepo.EnqueuedRows())

	// 2. Update to PAID status
	_, err = orderRepo.UpdateOrderStatus(ctx, created.ID, repository.OrderStatusPaid, "")
	require.NoError(t, err)

	rows := outboxRepo.EnqueuedRows()
	require.Len(t, rows, 1)
	assert.Equal(t, "order-123", rows[0].AggregateID)
	assert.Equal(t, OrderPaidEventType, rows[0].EventType)

	// 3. Relayer sweep claims and publishes
	count, err := relayer.SweepClaims(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	pub.mu.Lock()
	defer pub.mu.Unlock()
	require.Len(t, pub.published, 1)
	assert.Equal(t, OrderEventsTopic, pub.published[0].topic)
	assert.Equal(t, "order-123", pub.published[0].key)
}
