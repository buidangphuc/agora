// Package events consumes the listing events that keep the OpenSearch read-model
// in sync (ADR-0002/0005): the SDK consume loop plus the listing event handler.
package events

import (
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
)

// DLQSuffix is appended to the source topic to name its dead-letter topic.
const DLQSuffix = ".dlq"

// Consumer runs the SDK consume loop over a consumer-group client. A record is
// committed only after the handler accepts it or, once its retries are
// exhausted, after it is parked on the dead-letter topic (AD1); if it can be
// neither handled nor parked, Run returns the error without committing.
type Consumer struct {
	loop   *consume.Consumer
	client *kgo.Client
	closer func()
}

// NewConsumer joins group on topic and parks exhausted records on topic+".dlq".
// o tunes retries; its DLQTopic defaults to topic+DLQSuffix.
func NewConsumer(k config.Kafka, group, topic string, h Handler, o consume.Options) (*Consumer, error) {
	client, err := kafkax.NewConsumerClient(k, group, []string{topic})
	if err != nil {
		return nil, fmt.Errorf("kafka consumer: %w", err)
	}
	dlq, err := kafkax.NewProducer(k)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("kafka dlq producer: %w", err)
	}
	if o.DLQTopic == "" {
		o.DLQTopic = topic + DLQSuffix
	}
	c := NewConsumerWithDLQ(client, dlq, h, o)
	c.closer = dlq.Close
	return c, nil
}

// NewConsumerWithDLQ is NewConsumer over a caller-built client (auto-commit
// disabled) and dead-letter producer, for tests that need to fake the DLQ.
func NewConsumerWithDLQ(client *kgo.Client, dlq consume.DLQProducer, h Handler, o consume.Options) *Consumer {
	return &Consumer{
		loop:   consume.NewConsumer(client, adapt(h), dlq, o),
		client: client,
	}
}

// Run polls until ctx ends (it then returns nil) or a record cannot be resolved.
func (c *Consumer) Run(ctx context.Context) error { return c.loop.Run(ctx) }

// Close closes the consumer client and the dead-letter producer.
func (c *Consumer) Close() {
	c.client.Close()
	if c.closer != nil {
		c.closer()
	}
}

// adapt turns a key/value handler into an SDK consume.Handler.
func adapt(h Handler) consume.Handler {
	return func(ctx context.Context, r consume.Record) error { return h(ctx, r.Key, r.Value) }
}
