// Package events builds and relays team-payment domain events through the
// transactional outbox (ADR-0002 / ADR-0009). Payment settlement emits a
// platform.payment.v1.PaymentSettled wrapped in a platform.events.v1.EventEnvelope
// on the "payment.events" topic, keyed by order_id for per-order ordering.
package events

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/buidangphuc/team-payment/generated/platform/events/v1"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
)

// PaymentSettledEventType is the exported discriminator stamped on the outbox
// row and the EventEnvelope, so the settle path and the store agree on one
// source of truth. Consumers switch on it.
const PaymentSettledEventType = "platform.payment.v1.PaymentSettled"

// PaymentEventsTopic is the Kafka topic the relayer produces settlement events to.
const PaymentEventsTopic = "payment.events"

// BuildPaymentSettledEnvelope marshals the EventEnvelope carrying a PaymentSettled
// event. eventID is the outbox row id (a STABLE event_id that survives
// re-delivery under at-least-once, so idempotent consumers can dedupe). The
// returned bytes are stored verbatim in the outbox and produced unchanged by the
// relayer.
func BuildPaymentSettledEnvelope(
	eventID string,
	paymentID, orderID, buyerID string,
	status paymentv1.PaymentStatus,
	occurredAt time.Time,
	requestID string,
) ([]byte, error) {
	payload, err := proto.Marshal(&paymentv1.PaymentSettled{
		PaymentId:  paymentID,
		OrderId:    orderID,
		BuyerId:    buyerID,
		Status:     status,
		OccurredAt: occurredAt.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal PaymentSettled: %w", err)
	}
	envelope := &eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       PaymentSettledEventType,
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
