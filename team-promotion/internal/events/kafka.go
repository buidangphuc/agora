package events

import (
	"context"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
)

// KafkaPublisher publishes keyed records to one topic through the SDK producer.
// kafkax adds the traceparent and x-request-id record headers; the envelope
// carries its own traceparent as well, for consumers that read only the value.
type KafkaPublisher struct {
	producer *kafkax.Producer
	topic    string
}

// NewKafkaPublisher builds the producer for brokers in c and binds it to topic.
func NewKafkaPublisher(c config.Kafka, topic string) (*KafkaPublisher, error) {
	p, err := kafkax.NewProducer(c)
	if err != nil {
		return nil, err
	}
	return &KafkaPublisher{producer: p, topic: topic}, nil
}

// Publish produces one record synchronously and returns the produce error.
func (k *KafkaPublisher) Publish(ctx context.Context, key string, value []byte) error {
	return k.producer.Publish(ctx, k.topic, []byte(key), value)
}

// Close flushes buffered records and closes the client.
func (k *KafkaPublisher) Close(context.Context) error {
	k.producer.Close()
	return nil
}
