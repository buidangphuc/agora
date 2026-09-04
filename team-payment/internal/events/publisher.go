// Package events builds and relays team-payment domain events through the
// transactional outbox (ADR-0002 / ADR-0009). Payment settlement emits a
// platform.payment.v1.PaymentSettled wrapped in a platform.events.v1.EventEnvelope
// on the "payment.events" topic, keyed by order_id for per-order ordering.
package events

import (
	"context"
	"fmt"
	"log/slog"
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

// RawProducer produces already-marshalled EventEnvelope bytes keyed for
// ordering. The relayer depends on this interface; a real Kafka client (e.g.
// franz-go) satisfies it in a later wiring step.
type RawProducer interface {
	ProduceRaw(ctx context.Context, key string, payload []byte) error
}

// LogProducer is the default RawProducer: it logs each produce instead of
// talking to a broker. It keeps the outbox+relayer path exercisable without
// pulling in a Kafka client dependency; swap it for a broker-backed producer
// when Kafka is wired.
type LogProducer struct {
	logger *slog.Logger
	topic  string
}

// NewLogProducer returns a producer that logs to logger for topic.
func NewLogProducer(logger *slog.Logger, topic string) *LogProducer {
	if logger == nil {
		logger = slog.Default()
	}
	if topic == "" {
		topic = PaymentEventsTopic
	}
	return &LogProducer{logger: logger, topic: topic}
}

// ProduceRaw records the intended produce; it always succeeds.
func (p *LogProducer) ProduceRaw(ctx context.Context, key string, payload []byte) error {
	p.logger.InfoContext(ctx, "outbox produce (log-only)",
		slog.String("topic", p.topic),
		slog.String("key", key),
		slog.Int("bytes", len(payload)),
	)
	return nil
}

var _ RawProducer = (*LogProducer)(nil)
