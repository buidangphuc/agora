package events

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/repository"
)

// OrderPaidEventType is the fully-qualified proto type for OrderPaidEvent.
const OrderPaidEventType = "platform.order.v1.OrderPaidEvent"

// OrderShippedEventType is the fully-qualified proto type for OrderShipped.
const OrderShippedEventType = "platform.order.v1.OrderShipped"

// OrderCancelledEventType is the fully-qualified proto type for OrderCancelled.
const OrderCancelledEventType = "platform.order.v1.OrderCancelled"

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

// orderPaidNamespace seeds deterministic event ids so a re-emitted PAID fact for
// the same order collapses onto one outbox row (ON CONFLICT DO NOTHING) and one
// consumer-side dedupe key.
var orderPaidNamespace = uuid.NewSHA1(uuid.NameSpaceURL, []byte("agora/team-order/order.events/OrderPaid"))

// OrderPaidEventID is the stable EventEnvelope.event_id for an order's PAID fact.
func OrderPaidEventID(orderID string) string {
	return uuid.NewSHA1(orderPaidNamespace, []byte(orderID)).String()
}

// BuildPaidOutboxRow is the repository.PaidOutboxBuilder for team-order: it wraps
// the order's OrderPaidEvent (with line items) in an EventEnvelope, keyed by
// order_id, ready to commit in the same transaction as the PAID transition.
// order.UpdatedAt is the transition time and becomes occurred_at / paid_at.
func BuildPaidOutboxRow(order repository.Order) (repository.OutboxRow, error) {
	occurredAt := order.UpdatedAt
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}
	eventID := OrderPaidEventID(order.ID)
	payload, err := BuildOrderPaidEnvelope(eventID, order, occurredAt.UTC(), "")
	if err != nil {
		return repository.OutboxRow{}, err
	}
	return repository.OutboxRow{
		EventID:       eventID,
		AggregateType: "Order",
		AggregateID:   order.ID,
		EventType:     OrderPaidEventType,
		Payload:       payload,
	}, nil
}

// orderCancelledNamespace seeds deterministic OrderCancelled event ids. An order
// is cancelled at most once (the claim is a compare-and-set out of Pending/Paid
// into a terminal status), so one id per order is stable across redelivery.
var orderCancelledNamespace = uuid.NewSHA1(uuid.NameSpaceURL, []byte("agora/team-order/order.events/OrderCancelled"))

// OrderCancelledEventID is the stable EventEnvelope.event_id for an order's CANCELLED fact.
func OrderCancelledEventID(orderID string) string {
	return uuid.NewSHA1(orderCancelledNamespace, []byte(orderID)).String()
}

// BuildCancelledOutboxRow is the repository.CancelledOutboxBuilder for team-order:
// it wraps OrderCancelled in an EventEnvelope keyed by order_id, ready to commit in
// the same transaction as the winning claim to CANCELLED. order is the row the
// claim returned: previous_status is PAID iff paid_at is set (paid_at is written
// only by the move to Paid, and only Pending or Paid orders can be cancelled),
// otherwise PENDING. order.UpdatedAt is the claim time and becomes cancelled_at.
func BuildCancelledOutboxRow(order repository.Order) (repository.OutboxRow, error) {
	cancelledAt := order.UpdatedAt
	if cancelledAt.IsZero() {
		cancelledAt = time.Now()
	}
	cancelledAt = cancelledAt.UTC()
	previous := orderv1.OrderStatus_ORDER_STATUS_PENDING
	if order.PaidAt != nil {
		previous = orderv1.OrderStatus_ORDER_STATUS_PAID
	}
	eventID := OrderCancelledEventID(order.ID)
	payload, err := proto.Marshal(&orderv1.OrderCancelled{
		OrderId:        order.ID,
		BuyerId:        order.BuyerID,
		SellerId:       order.SellerID,
		PreviousStatus: previous,
		TotalAmount:    order.TotalAmount,
		Currency:       order.Currency,
		CancelledAt:    timestamppb.New(cancelledAt),
	})
	if err != nil {
		return repository.OutboxRow{}, fmt.Errorf("marshal OrderCancelled: %w", err)
	}
	value, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       OrderCancelledEventType,
		OccurredAt: timestamppb.New(cancelledAt),
		Payload:    payload,
	})
	if err != nil {
		return repository.OutboxRow{}, fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	return repository.OutboxRow{
		EventID:       eventID,
		AggregateType: "Order",
		AggregateID:   order.ID,
		EventType:     OrderCancelledEventType,
		Payload:       value,
	}, nil
}

// orderShippedNamespace seeds deterministic OrderShipped event ids (one per shipment).
var orderShippedNamespace = uuid.NewSHA1(uuid.NameSpaceURL, []byte("agora/team-order/order.events/OrderShipped"))

// OrderShippedEventID is the stable EventEnvelope.event_id for a shipment's OrderShipped fact.
func OrderShippedEventID(shipmentID string) string {
	return uuid.NewSHA1(orderShippedNamespace, []byte(shipmentID)).String()
}

// BuildShippedOutboxRow is the repository.ShipmentOutboxBuilder for team-order: it
// wraps OrderShipped in an EventEnvelope keyed by order_id, ready to commit in the
// same transaction as the shipment. s.BuyerID/SellerID come from the order.
func BuildShippedOutboxRow(s repository.Shipment) (repository.OutboxRow, error) {
	shippedAt := s.CreatedAt
	if shippedAt.IsZero() {
		shippedAt = time.Now()
	}
	shippedAt = shippedAt.UTC()
	eventID := OrderShippedEventID(s.ID)
	payload, err := proto.Marshal(&orderv1.OrderShipped{
		OrderId:      s.OrderID,
		BuyerId:      s.BuyerID,
		SellerId:     s.SellerID,
		Carrier:      s.Carrier,
		TrackingCode: s.TrackingCode,
		ShippedAt:    timestamppb.New(shippedAt),
	})
	if err != nil {
		return repository.OutboxRow{}, fmt.Errorf("marshal OrderShipped: %w", err)
	}
	value, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       OrderShippedEventType,
		OccurredAt: timestamppb.New(shippedAt),
		Payload:    payload,
	})
	if err != nil {
		return repository.OutboxRow{}, fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	return repository.OutboxRow{
		EventID:       eventID,
		AggregateType: "Order",
		AggregateID:   s.OrderID,
		EventType:     OrderShippedEventType,
		Payload:       value,
	}, nil
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
