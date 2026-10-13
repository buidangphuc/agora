package kafkax

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
)

// A client is built without dialing, so these tests need no broker.
const unreachable = "127.0.0.1:1"

func TestProducerDefaultsRecordDeliveryTimeout(t *testing.T) {
	p, err := NewProducer(config.Kafka{Brokers: unreachable})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if got := p.client.OptValue(kgo.RecordDeliveryTimeout); got != DefaultDeliveryTimeout || DefaultDeliveryTimeout != 30*time.Second {
		t.Fatalf("RecordDeliveryTimeout = %v, want 30s", got)
	}

	o, err := NewProducer(config.Kafka{Brokers: unreachable}, kgo.RecordDeliveryTimeout(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if got := o.client.OptValue(kgo.RecordDeliveryTimeout); got != 5*time.Second {
		t.Fatalf("overridden RecordDeliveryTimeout = %v, want 5s", got)
	}
}

func TestGroupConsumerClientBlocksRebalanceOnPoll(t *testing.T) {
	c, err := NewConsumerClient(config.Kafka{Brokers: unreachable}, "g", []string{"t"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if got := c.OptValue(kgo.BlockRebalanceOnPoll); got != true {
		t.Fatalf("BlockRebalanceOnPoll = %v, want true", got)
	}
	if got := c.OptValue(kgo.ConsumerGroup); got != "g" {
		t.Fatalf("ConsumerGroup = %v", got)
	}
}

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func TestCheckerReportsPingResult(t *testing.T) {
	if err := Checker(fakePinger{})(context.Background()); err != nil {
		t.Fatalf("healthy: %v", err)
	}
	down := errors.New("no brokers")
	if err := Checker(fakePinger{err: down})(context.Background()); !errors.Is(err, down) {
		t.Fatalf("down: %v, want the ping error", err)
	}
}
