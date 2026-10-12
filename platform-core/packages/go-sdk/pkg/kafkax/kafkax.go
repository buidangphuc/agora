package kafkax

import (
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/plugin/kotel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"
)

const (
	// HeaderTraceparent is the W3C trace-context record header.
	HeaderTraceparent = "traceparent"
	// HeaderRequestID carries the request id from reqctx.
	HeaderRequestID = reqctx.MetadataRequestID
)

var propagator = propagation.TraceContext{}

// Producer is a synchronous franz-go producer.
type Producer struct {
	client *kgo.Client
}

// NewProducer dials the configured brokers. opts are applied last, so they
// override the defaults (linger 0, kotel hooks), for example
// kgo.RecordDeliveryTimeout.
func NewProducer(c config.Kafka, opts ...kgo.Opt) (*Producer, error) {
	base := []kgo.Opt{
		kgo.SeedBrokers(c.BrokerList()...),
		kgo.ProducerLinger(0),
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

// Close flushes buffered records and closes the client.
func (p *Producer) Close() { p.client.Close() }

// NewConsumerClient joins group on topics with auto-commit disabled, so the
// caller commits offsets only after a record is handled or dead-lettered.
func NewConsumerClient(c config.Kafka, group string, topics ...string) (*kgo.Client, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(c.BrokerList()...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topics...),
		kgo.DisableAutoCommit(),
		kgo.WithHooks(hooks(group)...),
	)
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
