// Package events is the gateway's minimal Kafka producer for edge telemetry.
// It carries a single concern: wrap a platform.analytics.v1.TrackingEvent in a
// platform.events.v1.EventEnvelope and produce it on the `analytics.events`
// topic (ADR-0002, ADR-0005). No business logic lives here — the collector
// validates + stamps identity, this only marshals + produces (Rule 2). It
// mirrors team-domain/internal/events/publisher.go (the proven publish pattern).
package events

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"

	analyticsv1 "github.com/buidangphuc/team-gateway/generated/platform/analytics/v1"
	commonv1 "github.com/buidangphuc/team-gateway/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-gateway/generated/platform/events/v1"
)

// trackingEventType is the discriminator carried in EventEnvelope.Type so
// consumers switch on it and unmarshal the payload as a TrackingEvent.
const trackingEventType = "platform.analytics.v1.TrackingEvent"

// AnalyticsPublisher emits behavioral tracking events. The collector depends on
// this interface so tests can swap the Kafka client for a fake.
type AnalyticsPublisher interface {
	// PublishTrackingEvent wraps ev in an EventEnvelope (stamping principal +
	// requestID) and produces it to the analytics topic. The Kafka record key is
	// the session id (falling back to the anonymous id) so a visitor's activity
	// keeps per-session ordering on the topic. A non-empty eventID becomes the
	// envelope event_id (deterministic, so a re-sent beacon dedupes downstream);
	// an empty one keeps a freshly minted random UUID.
	PublishTrackingEvent(ctx context.Context, ev *analyticsv1.TrackingEvent, principal *commonv1.Principal, requestID, eventID string) error
	Close()
}

// KafkaPublisher produces to a Kafka/Redpanda topic through a go-sdk kafkax
// producer (synchronous, with trace-context and request-id record headers).
type KafkaPublisher struct {
	producer *kafkax.Producer
	topic    string
}

// NewKafkaPublisher dials the brokers and returns a publisher for topic.
func NewKafkaPublisher(c sdkconfig.Kafka, topic string) (*KafkaPublisher, error) {
	producer, err := kafkax.NewProducer(c)
	if err != nil {
		return nil, err
	}
	return &KafkaPublisher{producer: producer, topic: topic}, nil
}

// PublishTrackingEvent serializes the tracking event, wraps it in an
// EventEnvelope, and produces it synchronously.
func (p *KafkaPublisher) PublishTrackingEvent(
	ctx context.Context,
	ev *analyticsv1.TrackingEvent,
	principal *commonv1.Principal,
	requestID string,
	eventID string,
) error {
	if eventID == "" {
		eventID = uuid.NewString()
	}
	payload, err := proto.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal TrackingEvent: %w", err)
	}
	envelope := &eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       trackingEventType,
		OccurredAt: timestamppb.Now(),
		Principal:  principal,
		RequestId:  requestID,
		Payload:    payload,
	}
	value, err := proto.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	if err := p.producer.Publish(ctx, p.topic, []byte(eventKey(ev)), value); err != nil {
		return fmt.Errorf("produce %s: %w", trackingEventType, err)
	}
	return nil
}

// Close flushes and shuts down the client.
func (p *KafkaPublisher) Close() { p.producer.Close() }

// eventKey picks the partition key: session id first (groups a visitor's burst),
// then anonymous id, then the listing id as a last resort.
func eventKey(ev *analyticsv1.TrackingEvent) string {
	if k := ev.GetSessionId(); k != "" {
		return k
	}
	if k := ev.GetAnonymousId(); k != "" {
		return k
	}
	return ev.GetListingId()
}

// compile-time assertions.
var _ AnalyticsPublisher = (*KafkaPublisher)(nil)
