package events

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/repository"
)

// OrderPaidEventType is the fully-qualified proto type for OrderPaidEvent.
const OrderPaidEventType = "platform.order.v1.OrderPaidEvent"

// OrderEventsTopic is the Kafka topic for order lifecycle events.
const OrderEventsTopic = "order.events"

// BuildOrderPaidEnvelope constructs the EventEnvelope carrying OrderPaidEvent with line items.
func BuildOrderPaidEnvelope(
	eventID string,
	order repository.Order,
	occurredAt time.Time,
	requestID string,
) ([]byte, error) {
	items := make([]*orderv1.OrderLineItemFact, len(order.Items))
	for i, it := range order.Items {
		sellerID := order.SellerID
		items[i] = &orderv1.OrderLineItemFact{
			ListingId: it.ListingID,
			VariantId: it.VariantID,
			SellerId:  sellerID,
			Quantity:  it.Quantity,
			UnitPrice: it.UnitPrice,
			Currency:  order.Currency,
		}
	}

	payload, err := proto.Marshal(&orderv1.OrderPaidEvent{
		OrderId:     order.ID,
		BuyerId:     order.BuyerID,
		Items:       items,
		TotalAmount: order.TotalAmount,
		Currency:    order.Currency,
		PaidAt:      timestamppb.New(occurredAt),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal OrderPaidEvent: %w", err)
	}

	envelope := &eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       OrderPaidEventType,
		OccurredAt: timestamppb.New(occurredAt),
		RequestId:  requestID,
		Payload:    payload,
	}
	value, err := proto.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	return value, nil
}

// KafkaPublisher sends byte payloads to a Kafka topic.
type KafkaPublisher interface {
	Publish(ctx context.Context, topic, key string, payload []byte) error
}

// LoggingPublisher logs published events; used in testing and local fallback.
type LoggingPublisher struct {
	logger *slog.Logger
}

// NewLoggingPublisher constructs a logging publisher.
func NewLoggingPublisher(logger *slog.Logger) *LoggingPublisher {
	return &LoggingPublisher{logger: logger}
}

func (p *LoggingPublisher) Publish(ctx context.Context, topic, key string, payload []byte) error {
	p.logger.Info("relaying outbox event", "topic", topic, "key", key, "bytes", len(payload))
	return nil
}
