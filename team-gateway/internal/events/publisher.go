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
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	analyticsv1 "github.com/buidangphuc/team-gateway/generated/platform/analytics/v1"
	commonv1 "github.com/buidangphuc/team-gateway/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-gateway/generated/platform/events/v1"
)

// trackingEventType is the discriminator carried in EventEnvelope.Type so
// consumers switch on it and unmarshal the payload as a TrackingEvent.
const trackingEventType = "platform.analytics.v1.TrackingEvent"

// AnalyticsPublisher emits behavioral tracking events. The collector depends on
// this interface so the Kafka client can be swapped for a no-op when disabled.
type AnalyticsPublisher interface {
	// PublishTrackingEvent wraps ev in an EventEnvelope (stamping principal +
	// requestID) and produces it to the analytics topic. The Kafka record key is
	// the session id (falling back to the anonymous id) so a visitor's activity
	// keeps per-session ordering on the topic.
	PublishTrackingEvent(ctx context.Context, ev *analyticsv1.TrackingEvent, principal *commonv1.Principal, requestID string) error
	Close()
}

// NoopPublisher is used when KAFKA_ENABLED=false: the beacon still succeeds,
// nothing is emitted. Telemetry is best-effort, so this never errors.
type NoopPublisher struct{}

func (NoopPublisher) PublishTrackingEvent(context.Context, *analyticsv1.TrackingEvent, *commonv1.Principal, string) error {
	return nil
}
func (NoopPublisher) Close() {}

// KafkaPublisher produces to a Kafka/Redpanda topic via franz-go.
type KafkaPublisher struct {
	client *kgo.Client
	topic  string
}

// NewKafkaPublisher dials the brokers and returns a publisher for topic.
func NewKafkaPublisher(brokers []string, topic string) (*KafkaPublisher, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ProducerLinger(0),
	)
	if err != nil {
		return nil, fmt.Errorf("kafka client: %w", err)
	}
	return &KafkaPublisher{client: client, topic: topic}, nil
}

// PublishTrackingEvent serializes the tracking event, wraps it in an
// EventEnvelope, and produces it synchronously.
func (p *KafkaPublisher) PublishTrackingEvent(
	ctx context.Context,
	ev *analyticsv1.TrackingEvent,
	principal *commonv1.Principal,
	requestID string,
) error {
	payload, err := proto.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal TrackingEvent: %w", err)
	}
	envelope := &eventsv1.EventEnvelope{
		EventId:    uuid.NewString(),
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
	rec := &kgo.Record{Topic: p.topic, Key: []byte(eventKey(ev)), Value: value}
	if err := p.client.ProduceSync(ctx, rec).FirstErr(); err != nil {
		return fmt.Errorf("produce %s to %s: %w", trackingEventType, p.topic, err)
	}
	return nil
}

// Close flushes and shuts down the client.
func (p *KafkaPublisher) Close() { p.client.Close() }

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
var (
	_ AnalyticsPublisher = (*KafkaPublisher)(nil)
	_ AnalyticsPublisher = NoopPublisher{}
)
