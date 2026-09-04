// Package events publishes domain events to Kafka (ADR-0002). team-chat emits
// chat events wrapped in a platform.events.v1.EventEnvelope, keyed by thread id for per-thread ordering.
package events

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatv1 "github.com/buidangphuc/team-chat/generated/platform/chat/v1"
	commonv1 "github.com/buidangphuc/team-chat/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-chat/generated/platform/events/v1"
)

// Discriminator types carried in EventEnvelope.Type
const (
	ChatMessageSentType = "platform.chat.v1.ChatMessage"
)

// ChatPublisher emits domain events. Handlers depend on this interface.
type ChatPublisher interface {
	PublishMessageSent(ctx context.Context, msg *chatv1.ChatMessage, principal *commonv1.Principal, requestID string) error
	Close()
}

// NoopPublisher is used when KAFKA_ENABLED=false: writes still succeed, nothing is emitted.
type NoopPublisher struct{}

func (NoopPublisher) PublishMessageSent(context.Context, *chatv1.ChatMessage, *commonv1.Principal, string) error {
	return nil
}

func (NoopPublisher) Close() {}

// KafkaPublisher publishes to a Kafka/Redpanda topic via franz-go.
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

func (p *KafkaPublisher) produceEnvelope(ctx context.Context, key, eventType string, payload []byte, principal *commonv1.Principal, requestID string) error {
	envelope := &eventsv1.EventEnvelope{
		EventId:    uuid.NewString(),
		Type:       eventType,
		OccurredAt: timestamppb.Now(),
		Principal:  principal,
		RequestId:  requestID,
		Payload:    payload,
	}
	value, err := proto.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	rec := &kgo.Record{Topic: p.topic, Key: []byte(key), Value: value}
	if err := p.client.ProduceSync(ctx, rec).FirstErr(); err != nil {
		return fmt.Errorf("produce %s to %s: %w", eventType, p.topic, err)
	}
	return nil
}

// PublishMessageSent wraps the ChatMessage in an EventEnvelope and produces it synchronously.
func (p *KafkaPublisher) PublishMessageSent(
	ctx context.Context,
	msg *chatv1.ChatMessage,
	principal *commonv1.Principal,
	requestID string,
) error {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal ChatMessage: %w", err)
	}
	return p.produceEnvelope(ctx, msg.GetThreadId(), ChatMessageSentType, payload, principal, requestID)
}

// Close flushes and shuts down the client.
func (p *KafkaPublisher) Close() {
	if p.client != nil {
		p.client.Close()
	}
}

// compile-time assertions.
var (
	_ ChatPublisher = (*KafkaPublisher)(nil)
	_ ChatPublisher = NoopPublisher{}
)
