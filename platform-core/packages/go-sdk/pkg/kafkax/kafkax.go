package kafkax

import (
	"context"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/plugin/kotel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
)

// Record header names. They are the one set every package of the SDK uses
// (the outbox relayer stamps the first four, the consumers read them and stamp
// the dead-letter ones), all lower-case kebab-case.
const (
	// HeaderEventID carries the id of the event an outbox record was built from.
	HeaderEventID = "event-id"
	// HeaderEventType carries the type of that event.
	HeaderEventType = "event-type"
	// HeaderRequestID carries the request id from reqctx.
	HeaderRequestID = reqctx.MetadataRequestID
	// HeaderTraceparent is the W3C trace-context record header.
	HeaderTraceparent = "traceparent"

	// HeaderDLQError carries the last handler error on a dead-letter record.
	HeaderDLQError = "dlq-error"
	// HeaderDLQSourceTopic carries the topic a dead-letter record came from.
	HeaderDLQSourceTopic = "dlq-source-topic"
	// HeaderDLQSourcePartition carries the partition a dead-letter record came from.
	HeaderDLQSourcePartition = "dlq-source-partition"
	// HeaderDLQSourceOffset carries the offset a dead-letter record had on its source partition.
	HeaderDLQSourceOffset = "dlq-source-offset"
	// HeaderDLQSourceGroup carries the consumer group that dead-lettered the record.
	HeaderDLQSourceGroup = "dlq-source-group"
)

// DefaultDeliveryTimeout bounds how long a produced record may wait for a broker
// ack before Publish fails, so a broker outage cannot block callers (the outbox
// relayer among them) forever.
const DefaultDeliveryTimeout = 30 * time.Second

var propagator = propagation.TraceContext{}

// Producer is a synchronous franz-go producer.
type Producer struct {
	client *kgo.Client
}

// NewProducer dials the configured brokers. opts are applied last, so they
// override the defaults (linger 0, kotel hooks, a DefaultDeliveryTimeout record
// delivery timeout), for example kgo.RecordDeliveryTimeout.
func NewProducer(c config.Kafka, opts ...kgo.Opt) (*Producer, error) {
	base := []kgo.Opt{
		kgo.SeedBrokers(c.BrokerList()...),
		kgo.ProducerLinger(0),
		kgo.RecordDeliveryTimeout(DefaultDeliveryTimeout),
		kgo.WithHooks(hooks("")...),
	}
	client, err := kgo.NewClient(append(base, opts...)...)
	if err != nil {
		return nil, fmt.Errorf("kafkax: producer client: %w", err)
	}
	return &Producer{client: client}, nil
}

// Publish produces one record and waits for the broker ack. It adds the
// traceparent and x-request-id headers from ctx and returns the produce error.
func (p *Producer) Publish(ctx context.Context, topic string, key, value []byte) error {
	return p.PublishRecord(ctx, &kgo.Record{Topic: topic, Key: key, Value: value})
}

// PublishRecord is Publish for a caller-built record, such as a dead-letter
// record with its own headers. Headers already on rec are kept, and the
// traceparent and x-request-id headers are set from ctx unless rec has them.
func (p *Producer) PublishRecord(ctx context.Context, rec *kgo.Record) error {
	rec.Context = ctx
	if !hasHeader(rec, HeaderTraceparent) {
		propagator.Inject(ctx, kotel.NewRecordCarrier(rec))
	}
	if id := reqctx.RequestID(ctx); id != "" && !hasHeader(rec, HeaderRequestID) {
		rec.Headers = append(rec.Headers, kgo.RecordHeader{Key: HeaderRequestID, Value: []byte(id)})
	}
	if err := p.client.ProduceSync(ctx, rec).FirstErr(); err != nil {
		return fmt.Errorf("kafkax: produce to %s: %w", rec.Topic, err)
	}
	return nil
}

// Ping checks that a broker answers. Use it with Checker.
func (p *Producer) Ping(ctx context.Context) error { return p.client.Ping(ctx) }

// Close closes the client. Publish is synchronous and returns only once the
// record is acked or failed, so there is nothing buffered to flush; call Close
// after the last Publish has returned.
func (p *Producer) Close() { p.client.Close() }

// Pinger is satisfied by *kgo.Client and *Producer.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Checker returns a health check that pings the brokers of c. It has the shape
// of healthx.Check, so healthx.Register("kafka", kafkax.Checker(client)) works.
func Checker(c Pinger) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		if err := c.Ping(ctx); err != nil {
			return fmt.Errorf("kafkax: ping: %w", err)
		}
		return nil
	}
}

// NewConsumerClient joins group on topics with auto-commit disabled, so the
// caller commits offsets only after a record is handled or dead-lettered, and
// with kgo.BlockRebalanceOnPoll, so a rebalance cannot revoke a partition
// between a poll and its commit. The caller must call Client.AllowRebalance
// after committing each fetch (and before Close, which waits for rebalances to
// be allowed); consume.Consumer and consume.BatchConsumer do.
// Handle each fetch within the group's rebalance timeout. opts are applied
// after the defaults, for example kgo.AllowAutoTopicCreation.
//
// With an empty group it builds a group-less client instead: it reads every
// partition of topics from the start, and nothing is committed, so no consumer
// group appears on the broker.
func NewConsumerClient(c config.Kafka, group string, topics []string, opts ...kgo.Opt) (*kgo.Client, error) {
	base := []kgo.Opt{
		kgo.SeedBrokers(c.BrokerList()...),
		kgo.ConsumeTopics(topics...),
		kgo.WithHooks(hooks(group)...),
	}
	if group == "" {
		base = append(base, kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	} else {
		base = append(base, kgo.ConsumerGroup(group), kgo.DisableAutoCommit(), kgo.BlockRebalanceOnPoll())
	}
	client, err := kgo.NewClient(append(base, opts...)...)
	if err != nil {
		return nil, fmt.Errorf("kafkax: consumer client: %w", err)
	}
	return client, nil
}

func hooks(group string) []kgo.Hook {
	topts := []kotel.TracerOpt{kotel.TracerPropagator(propagator)}
	if group != "" {
		topts = append(topts, kotel.ConsumerGroup(group))
	}
	k := kotel.NewKotel(kotel.WithTracer(kotel.NewTracer(topts...)), kotel.WithMeter(kotel.NewMeter()))
	return k.Hooks()
}

func hasHeader(rec *kgo.Record, key string) bool {
	for _, h := range rec.Headers {
		if h.Key == key {
			return true
		}
	}
	return false
}
