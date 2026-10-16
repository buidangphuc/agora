// Package events publishes identity's domain events to Kafka (ADR-0002, ADR-0003
// addendum). Events are wrapped in a platform.events.v1.EventEnvelope, keyed by user
// id for per-user ordering, and written through the transactional outbox: the
// relayer (relayer.go) drains stored rows to the topic via RawProducer.
package events

import (
	"context"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-identity/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
)

// SessionRevokedEventType is the discriminator carried in EventEnvelope.Type (and
// stamped on the outbox row) for the SessionRevoked event.
const SessionRevokedEventType = "platform.identity.v1.SessionRevoked"

// BuildSessionRevokedEnvelope marshals the EventEnvelope for a revoked session with
// a caller-supplied eventID (the outbox row id), so a re-delivered row carries a
// STABLE event_id. expiresAt is the revoked token's expiry: the gateway keeps the
// denylist entry until then.
func BuildSessionRevokedEnvelope(
	eventID, sessionID, userID string,
	expiresAt time.Time,
	principal *commonv1.Principal,
	requestID string,
) ([]byte, error) {
	payload, err := proto.Marshal(&identityv1.SessionRevoked{
		SessionId: sessionID,
		UserId:    userID,
		ExpiresAt: timestamppb.New(expiresAt),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal SessionRevoked: %w", err)
	}
	value, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       SessionRevokedEventType,
		OccurredAt: timestamppb.Now(),
		Principal:  principal,
		RequestId:  requestID,
		Payload:    payload,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	return value, nil
}

// UserStatusChangedEventType is the EventEnvelope.Type discriminator for the
// UserStatusChanged event.
const UserStatusChangedEventType = "platform.identity.v1.UserStatusChanged"

// BuildUserStatusChangedEnvelope marshals the EventEnvelope for a user status
// change. The payload carries only contract fields: there is deliberately no
// note and no username parameter. until may be nil.
func BuildUserStatusChangedEnvelope(
	eventID, userID string,
	status, previous identityv1.UserStatus,
	reasonCode string,
	until *time.Time,
	changedAt time.Time,
	principal *commonv1.Principal,
	requestID string,
) ([]byte, error) {
	ev := &identityv1.UserStatusChanged{
		UserId:         userID,
		Status:         status,
		PreviousStatus: previous,
		ReasonCode:     reasonCode,
		ChangedAt:      timestamppb.New(changedAt),
	}
	if until != nil {
		ev.Until = timestamppb.New(*until)
	}
	payload, err := proto.Marshal(ev)
	if err != nil {
		return nil, fmt.Errorf("marshal UserStatusChanged: %w", err)
	}
	value, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       UserStatusChangedEventType,
		OccurredAt: timestamppb.Now(),
		Principal:  principal,
		RequestId:  requestID,
		Payload:    payload,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	return value, nil
}

// Publisher is the lifecycle handle for the event producer: either a
// KafkaPublisher or a NoopPublisher, so bootstrap tears them down uniformly.
type Publisher interface {
	Close()
}

// NoopPublisher is used when KAFKA_ENABLED=false: revokes still succeed and their
// outbox rows are recorded, but nothing is ever produced.
type NoopPublisher struct{}

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

// ProduceRaw produces already-marshalled EventEnvelope bytes to the configured
// topic, keyed for per-user ordering. It is the relayer's produce path.
func (p *KafkaPublisher) ProduceRaw(ctx context.Context, key string, payload []byte) error {
	rec := &kgo.Record{Topic: p.topic, Key: []byte(key), Value: payload}
	if err := p.client.ProduceSync(ctx, rec).FirstErr(); err != nil {
		return fmt.Errorf("produce to %s: %w", p.topic, err)
	}
	return nil
}

// Close flushes and shuts down the client.
func (p *KafkaPublisher) Close() { p.client.Close() }

var (
	_ Publisher   = (*KafkaPublisher)(nil)
	_ Publisher   = NoopPublisher{}
	_ RawProducer = (*KafkaPublisher)(nil)
)
