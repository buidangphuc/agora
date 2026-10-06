// Package events publishes domain events to Kafka (ADR-0002, ADR-0013). team-chat
// writes a chat.events EventEnvelope to its outbox in the message transaction; a
// Relayer then produces it to Kafka keyed by thread id for per-thread ordering.
package events

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatv1 "github.com/buidangphuc/team-chat/generated/platform/chat/v1"
	eventsv1 "github.com/buidangphuc/team-chat/generated/platform/events/v1"
	"github.com/buidangphuc/team-chat/internal/interceptor"
	"github.com/buidangphuc/team-chat/internal/repository"
)

// Discriminator types carried in EventEnvelope.Type
const (
	ChatMessageSentType = "platform.chat.v1.ChatMessage"
)

// ChatEventsTopic is the default Kafka topic for chat events.
const ChatEventsTopic = "chat.events"

// chatMessageNamespace seeds deterministic event ids so a message maps onto exactly
// one outbox row (ON CONFLICT DO NOTHING) and one consumer-side dedupe key.
var chatMessageNamespace = uuid.NewSHA1(uuid.NameSpaceURL, []byte("agora/team-chat/chat.events/ChatMessage"))

// ChatMessageEventID is the stable EventEnvelope.event_id for a message's event.
func ChatMessageEventID(messageID string) string {
	return uuid.NewSHA1(chatMessageNamespace, []byte(messageID)).String()
}

// BuildMessageOutboxRow is the repository.MessageOutboxBuilder for team-chat: it
// wraps the message (with recipient_id) in an EventEnvelope keyed by thread id,
// ready to commit in the same transaction as the message. The caller's principal
// and request id are read from ctx.
func BuildMessageOutboxRow(ctx context.Context, msg repository.ChatMessage) (repository.OutboxRow, error) {
	occurredAt := msg.CreatedAt
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}
	payload, err := proto.Marshal(&chatv1.ChatMessage{
		Id:          msg.ID,
		ThreadId:    msg.ThreadID,
		SenderId:    msg.SenderID,
		SenderName:  msg.SenderName,
		Content:     msg.Content,
		CreatedAt:   timestamppb.New(occurredAt),
		MessageType: chatv1.MessageType(msg.MessageType),
		ListingId:   msg.ListingID,
		Payload:     msg.Payload,
		RecipientId: msg.RecipientID,
		SellerId:    msg.SellerID,
	})
	if err != nil {
		return repository.OutboxRow{}, fmt.Errorf("marshal ChatMessage: %w", err)
	}
	reqID, _ := interceptor.RequestIDFromContext(ctx)
	eventID := ChatMessageEventID(msg.ID)
	envelope := &eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       ChatMessageSentType,
		OccurredAt: timestamppb.New(occurredAt.UTC()),
		RequestId:  reqID,
		Payload:    payload,
	}
	if p, ok := interceptor.PrincipalFromContext(ctx); ok {
		envelope.Principal = p
	}
	value, err := proto.Marshal(envelope)
	if err != nil {
		return repository.OutboxRow{}, fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	return repository.OutboxRow{
		EventID:       eventID,
		AggregateType: "ChatThread",
		AggregateID:   msg.ThreadID,
		EventType:     ChatMessageSentType,
		Payload:       value,
		RequestID:     reqID,
	}, nil
}

// KafkaPublisher sends byte payloads to a Kafka topic. The Relayer depends on it.
type KafkaPublisher interface {
	Publish(ctx context.Context, topic, key string, payload []byte) error
}

// KafkaProducer is the franz-go backed KafkaPublisher.
type KafkaProducer struct {
	client *kgo.Client
}

// NewKafkaProducer dials the brokers for producing.
func NewKafkaProducer(brokers []string) (*KafkaProducer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ProducerLinger(0),
	)
	if err != nil {
		return nil, fmt.Errorf("kafka client: %w", err)
	}
	return &KafkaProducer{client: client}, nil
}

// Publish produces payload to topic keyed by key and waits for the ack.
func (p *KafkaProducer) Publish(ctx context.Context, topic, key string, payload []byte) error {
	rec := &kgo.Record{Topic: topic, Key: []byte(key), Value: payload}
	if err := p.client.ProduceSync(ctx, rec).FirstErr(); err != nil {
		return fmt.Errorf("produce to %s: %w", topic, err)
	}
	return nil
}

// Close flushes and shuts the client down.
func (p *KafkaProducer) Close() {
	if p != nil && p.client != nil {
		p.client.Close()
	}
}

var _ KafkaPublisher = (*KafkaProducer)(nil)
