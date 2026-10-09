package events

import (
	"context"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// KafkaPublisher produces keyed records synchronously (acks=all).
type KafkaPublisher struct{ client *kgo.Client }

func NewKafkaPublisher(brokers []string) (*KafkaPublisher, error) {
	c, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ProducerLinger(0),
		kgo.RecordDeliveryTimeout(10*time.Second), // Kafka down: fail the cycle, retry next tick
	)
	if err != nil {
		return nil, err
	}
	return &KafkaPublisher{client: c}, nil
}

func (p *KafkaPublisher) Publish(ctx context.Context, topic, key string, value []byte) error {
	rec := &kgo.Record{Topic: topic, Key: []byte(key), Value: value}
	if err := p.client.ProduceSync(ctx, rec).FirstErr(); err != nil {
		return fmt.Errorf("produce to %s: %w", topic, err)
	}
	return nil
}

func (p *KafkaPublisher) Close() { p.client.Close() }
