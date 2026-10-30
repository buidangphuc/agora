package rpc_test

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	"github.com/buidangphuc/team-order/internal/store"
)

// OrderPaidEventType is the fully-qualified proto type for OrderPaidEvent.
const OrderPaidEventType = "platform.order.v1.OrderPaidEvent"

// OrderShippedEventType is the fully-qualified proto type for OrderShipped.
const OrderShippedEventType = "platform.order.v1.OrderShipped"

// OrderCancelledEventType is the fully-qualified proto type for OrderCancelled.
const OrderCancelledEventType = "platform.order.v1.OrderCancelled"

// ReturnRefundedEventType is the fully-qualified proto type for ReturnRefunded.
const ReturnRefundedEventType = "platform.order.v1.ReturnRefunded"

// ReturnRequestedEventType is the fully-qualified proto type for ReturnRequested.
const ReturnRequestedEventType = "platform.order.v1.ReturnRequested"

// OrderPlacedEventType is the fully-qualified proto type for OrderPlaced.
const OrderPlacedEventType = "platform.order.v1.OrderPlaced"

// OrderPaymentReminderEventType is the fully-qualified proto type for OrderPaymentReminder.
const OrderPaymentReminderEventType = "platform.order.v1.OrderPaymentReminder"

// OrderCompletedEventType is the fully-qualified proto type for OrderCompleted.
const OrderCompletedEventType = "platform.order.v1.OrderCompleted"

// BuildOrderPaidEnvelope constructs the EventEnvelope carrying OrderPaidEvent with line items.
func BuildOrderPaidEnvelope(
	eventID string,
	order store.Order,
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

	paid := &orderv1.OrderPaidEvent{
		OrderId:     order.ID,
		BuyerId:     order.BuyerID,
		SellerId:    order.SellerID,
		Items:       items,
		TotalAmount: order.TotalAmount,
		Currency:    order.Currency,
		PaidAt:      timestamppb.New(occurredAt),
	}
	if order.ShipBy != nil {
		paid.ShipBy = timestamppb.New(order.ShipBy.UTC())
	}
	payload, err := proto.Marshal(paid)
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

// BuildPaidOutboxRow is the store.PaidOutboxBuilder for team-order: it wraps
// the order's OrderPaidEvent (with line items) in an EventEnvelope, keyed by
// order_id, ready to commit in the same transaction as the PAID transition.
// order.UpdatedAt is the transition time and becomes occurred_at / paid_at.
func BuildPaidOutboxRow(order store.Order) (outbox.Row, error) {
	occurredAt := order.UpdatedAt
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}
	eventID := OrderPaidEventID(order.ID)
	payload, err := BuildOrderPaidEnvelope(eventID, order, occurredAt.UTC(), "")
	if err != nil {
		return outbox.Row{}, err
	}
	return outbox.Row{
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

// BuildCancelledOutboxRow is the store.CancelledOutboxBuilder for team-order:
// it wraps OrderCancelled in an EventEnvelope keyed by order_id, ready to commit in
// the same transaction as the winning claim to CANCELLED. order is the row the
// claim returned: previous_status is PAID iff paid_at is set (paid_at is written
// only by the move to Paid, and only Pending or Paid orders can be cancelled),
// otherwise PENDING. order.UpdatedAt is the claim time and becomes cancelled_at.
func BuildCancelledOutboxRow(order store.Order) (outbox.Row, error) {
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
		ReasonCode:     orderv1.CancelReason(order.CancelReason),
	})
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal OrderCancelled: %w", err)
	}
	value, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       OrderCancelledEventType,
		OccurredAt: timestamppb.New(cancelledAt),
		Payload:    payload,
	})
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	return outbox.Row{
		EventID:       eventID,
		AggregateType: "Order",
		AggregateID:   order.ID,
		EventType:     OrderCancelledEventType,
		Payload:       value,
	}, nil
}

// orderPlacedNamespace seeds deterministic OrderPlaced event ids (one per order).
var orderPlacedNamespace = uuid.NewSHA1(uuid.NameSpaceURL, []byte("agora/team-order/order.events/OrderPlaced"))

// OrderPlacedEventID is the stable EventEnvelope.event_id for an order's PLACED fact.
func OrderPlacedEventID(orderID string) string {
	return uuid.NewSHA1(orderPlacedNamespace, []byte(orderID)).String()
}

// BuildPlacedOutboxRow is the store.PlacedOutboxBuilder for team-order: it
// wraps OrderPlaced in an EventEnvelope keyed by order_id, ready to commit in the
// placement transaction. order.CreatedAt is placed_at; payment_due_at is the
// order's own (unset for COD); ship_by is the order's own (set for COD only).
func BuildPlacedOutboxRow(order store.Order) (outbox.Row, error) {
	placedAt := order.CreatedAt
	if placedAt.IsZero() {
		placedAt = time.Now()
	}
	placedAt = placedAt.UTC()
	ev := &orderv1.OrderPlaced{
		OrderId:       order.ID,
		BuyerId:       order.BuyerID,
		SellerId:      order.SellerID,
		TotalAmount:   order.TotalAmount,
		Currency:      order.Currency,
		PaymentMethod: paymentv1.PaymentMethod(order.PaymentMethod),
		PlacedAt:      timestamppb.New(placedAt),
	}
	if order.PaymentDueAt != nil {
		ev.PaymentDueAt = timestamppb.New(order.PaymentDueAt.UTC())
	}
	if order.ShipBy != nil {
		ev.ShipBy = timestamppb.New(order.ShipBy.UTC())
	}
	eventID := OrderPlacedEventID(order.ID)
	payload, err := proto.Marshal(ev)
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal OrderPlaced: %w", err)
	}
	value, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       OrderPlacedEventType,
		OccurredAt: timestamppb.New(placedAt),
		Payload:    payload,
	})
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	return outbox.Row{
		EventID:       eventID,
		AggregateType: "Order",
		AggregateID:   order.ID,
		EventType:     OrderPlacedEventType,
		Payload:       value,
	}, nil
}

// OrderCompletedEventID is the stable EventEnvelope.event_id for an order's
// COMPLETED fact; an order completes at most once.
func OrderCompletedEventID(orderID string) string { return "order-completed:" + orderID }

// BuildCompletedOutboxRow is the store.CompletedOutboxBuilder for team-order:
// it wraps OrderCompleted in an EventEnvelope keyed by order_id, committed in the
// same transaction as the winning move to COMPLETED. order is the row that move
// returned; paid_online is true iff the order has a paid_at.
func BuildCompletedOutboxRow(order store.Order) (outbox.Row, error) {
	completedAt := order.UpdatedAt
	if order.CompletedAt != nil {
		completedAt = *order.CompletedAt
	}
	if completedAt.IsZero() {
		completedAt = time.Now()
	}
	completedAt = completedAt.UTC()
	ev := &orderv1.OrderCompleted{
		OrderId:     order.ID,
		BuyerId:     order.BuyerID,
		SellerId:    order.SellerID,
		TotalAmount: order.TotalAmount,
		Currency:    order.Currency,
		CompletedBy: orderv1.CompletedBy(order.CompletedBy),
		PaidOnline:  order.PaidAt != nil,
		CompletedAt: timestamppb.New(completedAt),
	}
	if order.ReturnWindowEndsAt != nil {
		ev.ReturnWindowEndsAt = timestamppb.New(order.ReturnWindowEndsAt.UTC())
	}
	eventID := OrderCompletedEventID(order.ID)
	payload, err := proto.Marshal(ev)
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal OrderCompleted: %w", err)
	}
	value, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       OrderCompletedEventType,
		OccurredAt: timestamppb.New(completedAt),
		Payload:    payload,
	})
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	return outbox.Row{
		EventID:       eventID,
		AggregateType: "Order",
		AggregateID:   order.ID,
		EventType:     OrderCompletedEventType,
		Payload:       value,
	}, nil
}

// orderPaymentReminderNamespace seeds deterministic OrderPaymentReminder event ids
// (one per order: the reminder is emitted at most once).
var orderPaymentReminderNamespace = uuid.NewSHA1(uuid.NameSpaceURL, []byte("agora/team-order/order.events/OrderPaymentReminder"))

// OrderPaymentReminderEventID is the stable EventEnvelope.event_id for an order's reminder.
func OrderPaymentReminderEventID(orderID string) string {
	return uuid.NewSHA1(orderPaymentReminderNamespace, []byte(orderID)).String()
}

// BuildPaymentReminderOutboxRow is the store.ReminderOutboxBuilder for
// team-order: it wraps OrderPaymentReminder in an EventEnvelope keyed by order_id,
// committed in the same transaction as the payment_reminder_sent_at stamp.
// order.PaymentReminderSentAt is occurred_at; payment_due_at is the order's own.
func BuildPaymentReminderOutboxRow(order store.Order) (outbox.Row, error) {
	occurredAt := time.Now()
	if order.PaymentReminderSentAt != nil {
		occurredAt = *order.PaymentReminderSentAt
	}
	occurredAt = occurredAt.UTC()
	ev := &orderv1.OrderPaymentReminder{
		OrderId:     order.ID,
		BuyerId:     order.BuyerID,
		TotalAmount: order.TotalAmount,
		Currency:    order.Currency,
	}
	if order.PaymentDueAt != nil {
		ev.PaymentDueAt = timestamppb.New(order.PaymentDueAt.UTC())
	}
	eventID := OrderPaymentReminderEventID(order.ID)
	payload, err := proto.Marshal(ev)
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal OrderPaymentReminder: %w", err)
	}
	value, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       OrderPaymentReminderEventType,
		OccurredAt: timestamppb.New(occurredAt),
		Payload:    payload,
	})
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	return outbox.Row{
		EventID:       eventID,
		AggregateType: "Order",
		AggregateID:   order.ID,
		EventType:     OrderPaymentReminderEventType,
		Payload:       value,
	}, nil
}

// orderShippedNamespace seeds deterministic OrderShipped event ids (one per shipment).
var orderShippedNamespace = uuid.NewSHA1(uuid.NameSpaceURL, []byte("agora/team-order/order.events/OrderShipped"))

// OrderShippedEventID is the stable EventEnvelope.event_id for a shipment's OrderShipped fact.
func OrderShippedEventID(shipmentID string) string {
	return uuid.NewSHA1(orderShippedNamespace, []byte(shipmentID)).String()
}

// BuildShippedOutboxRow is the store.ShipmentOutboxBuilder for team-order: it
// wraps OrderShipped in an EventEnvelope keyed by order_id, ready to commit in the
// same transaction as the shipment. s.BuyerID/SellerID come from the order.
func BuildShippedOutboxRow(s store.Shipment) (outbox.Row, error) {
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
		return outbox.Row{}, fmt.Errorf("marshal OrderShipped: %w", err)
	}
	value, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       OrderShippedEventType,
		OccurredAt: timestamppb.New(shippedAt),
		Payload:    payload,
	})
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	return outbox.Row{
		EventID:       eventID,
		AggregateType: "Order",
		AggregateID:   s.OrderID,
		EventType:     OrderShippedEventType,
		Payload:       value,
	}, nil
}

// returnRefundedNamespace seeds deterministic ReturnRefunded event ids. A return
// reaches REFUNDED at most once (a compare-and-set out of APPROVED into a terminal
// status), so one id per return is stable across rebuilds and redelivery.
var returnRefundedNamespace = uuid.NewSHA1(uuid.NameSpaceURL, []byte("agora/team-order/order.events/ReturnRefunded"))

// ReturnRefundedEventID is the stable EventEnvelope.event_id for a return's REFUNDED fact.
func ReturnRefundedEventID(returnID string) string {
	return uuid.NewSHA1(returnRefundedNamespace, []byte(returnID)).String()
}

// BuildReturnRefundedOutboxRow is the store.ReturnOutboxBuilder for
// team-order: it wraps ReturnRefunded in an EventEnvelope keyed by order_id, ready
// to commit in the same transaction as the won APPROVED -> REFUNDED
// compare-and-set. ret is the row that CAS returned, so refund_amount is the
// stored return amount (never a client value) and ret.UpdatedAt the refund time;
// currency is the order's.
func BuildReturnRefundedOutboxRow(ret store.OrderReturn, currency string) (outbox.Row, error) {
	refundedAt := ret.UpdatedAt
	if refundedAt.IsZero() {
		refundedAt = time.Now()
	}
	refundedAt = refundedAt.UTC()
	eventID := ReturnRefundedEventID(ret.ID)
	payload, err := proto.Marshal(&orderv1.ReturnRefunded{
		ReturnId:     ret.ID,
		OrderId:      ret.OrderID,
		BuyerId:      ret.BuyerID,
		SellerId:     ret.SellerID,
		RefundAmount: ret.RefundAmount,
		Currency:     currency,
		RefundedAt:   timestamppb.New(refundedAt),
	})
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal ReturnRefunded: %w", err)
	}
	value, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       ReturnRefundedEventType,
		OccurredAt: timestamppb.New(refundedAt),
		Payload:    payload,
	})
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	return outbox.Row{
		EventID:       eventID,
		AggregateType: "Order",
		AggregateID:   ret.OrderID,
		EventType:     ReturnRefundedEventType,
		Payload:       value,
	}, nil
}

// ReturnRequestedEventID is the stable event_id of a return's ReturnRequested fact.
func ReturnRequestedEventID(returnID string) string { return "return-requested:" + returnID }

// BuildReturnRequestedOutboxRow is the store.ReturnOutboxBuilder for a
// created return: ReturnRequested in an EventEnvelope keyed by order_id.
func BuildReturnRequestedOutboxRow(ret store.OrderReturn, currency string) (outbox.Row, error) {
	requestedAt := ret.CreatedAt
	if requestedAt.IsZero() {
		requestedAt = time.Now()
	}
	requestedAt = requestedAt.UTC()
	eventID := ReturnRequestedEventID(ret.ID)
	payload, err := proto.Marshal(&orderv1.ReturnRequested{
		ReturnId:     ret.ID,
		OrderId:      ret.OrderID,
		BuyerId:      ret.BuyerID,
		SellerId:     ret.SellerID,
		RefundAmount: ret.RefundAmount,
		Currency:     currency,
		RequestedAt:  timestamppb.New(requestedAt),
	})
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal ReturnRequested: %w", err)
	}
	value, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       ReturnRequestedEventType,
		OccurredAt: timestamppb.New(requestedAt),
		Payload:    payload,
	})
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	return outbox.Row{
		EventID:       eventID,
		AggregateType: "Order",
		AggregateID:   ret.OrderID,
		EventType:     ReturnRequestedEventType,
		Payload:       value,
	}, nil
}
