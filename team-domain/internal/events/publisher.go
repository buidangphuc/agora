// Package events publishes domain events to Kafka (ADR-0002). team-domain emits
// domain events wrapped in a platform.events.v1.EventEnvelope, keyed by listing id for per-entity ordering.
package events

import (
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-domain/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-domain/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
)

// listingChangedType is the discriminator carried in EventEnvelope.Type for the
// ListingChanged event the outbox enqueues.
const listingChangedType = "platform.listing.v1.ListingChanged"

// ListingChangedEventType is the exported discriminator the outbox enqueue path
// stamps on the row (and the EventEnvelope), so the handler and the store agree
// on a single source of truth.
const ListingChangedEventType = listingChangedType

// BuildListingChangedEnvelope marshals the exact EventEnvelope the KafkaPublisher
// produced inline before the outbox — same fields, same inner ListingChanged —
// but with a caller-supplied eventID (the outbox row id) rather than a fresh
// UUID, so a re-delivered row carries a STABLE event_id every time. That
// stability is what lets idempotent consumers dedupe under at-least-once. The
// returned bytes are stored verbatim in the outbox and produced unchanged by the
// relayer, keeping the wire contract byte-identical (no consumer/proto change).
func BuildListingChangedEnvelope(
	eventID string,
	listing *listingv1.Listing,
	change listingv1.ChangeType,
	principal *commonv1.Principal,
	requestID string,
) ([]byte, error) {
	payload, err := proto.Marshal(&listingv1.ListingChanged{Listing: listing, ChangeType: change})
	if err != nil {
		return nil, fmt.Errorf("marshal ListingChanged: %w", err)
	}
	envelope := &eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       listingChangedType,
		OccurredAt: timestamppb.Now(),
		Principal:  principal,
		RequestId:  requestID,
		Payload:    payload,
	}
	value, err := proto.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	return value, nil
}

// ListingPublisher is the lifecycle handle for the event producer. Domain events
// are no longer published inline on the request path: every write records its
// event in the transactional outbox, and the relayer (internal/events/relayer.go)
// drains those rows to Kafka via the RawProducer.ProduceRaw path. This interface
// therefore only exposes Close, so bootstrap can hold either a real KafkaPublisher
// or a NoopPublisher and tear it down uniformly.
type ListingPublisher interface {
	Close()
}

// NoopPublisher is used when KAFKA_ENABLED=false: writes still succeed and their
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
// topic, keyed for per-entity ordering. It is the relayer's produce path: the
// outbox stored the whole envelope at enqueue time, so the relayer stays
// agnostic of event-type specifics and the wire bytes are byte-identical to the
// old inline produce.
func (p *KafkaPublisher) ProduceRaw(ctx context.Context, key string, payload []byte) error {
	rec := &kgo.Record{Topic: p.topic, Key: []byte(key), Value: payload}
	if err := p.client.ProduceSync(ctx, rec).FirstErr(); err != nil {
		return fmt.Errorf("produce to %s: %w", p.topic, err)
	}
	return nil
}

// Close flushes and shuts down the client.
func (p *KafkaPublisher) Close() { p.client.Close() }

// compile-time assertions.
var (
	_ ListingPublisher = (*KafkaPublisher)(nil)
	_ ListingPublisher = NoopPublisher{}
)
