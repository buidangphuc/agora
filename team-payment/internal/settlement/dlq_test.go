package settlement_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-payment/internal/settlement"
)

type recordingDLQ struct {
	mu   sync.Mutex
	fail bool
	keys []string
}

func (d *recordingDLQ) PublishRecord(_ context.Context, r *kgo.Record) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail {
		return errors.New("broker down")
	}
	d.keys = append(d.keys, string(r.Key))
	return nil
}

func (d *recordingDLQ) parked() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.keys...)
}

type creditApplier struct {
	mu      sync.Mutex
	credits []string
}

func (a *creditApplier) CreditSettlement(_ context.Context, orderID, _ string, _ int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.credits = append(a.credits, orderID)
	return nil
}
func (a *creditApplier) RefundCancelledOrder(context.Context, string) error        { return nil }
func (a *creditApplier) RefundReturn(context.Context, string, string, int64) error { return nil }

func (a *creditApplier) got() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.credits...)
}

// A record that is neither handled nor dead-lettered is never committed: the consumer
// stops with an error and the record is redelivered after a restart in the same group.
func TestConsumerDoesNotCommitOnDLQFailure(t *testing.T) {
	brokers := testkit.Redpanda(t)
	kc := sdkconfig.Kafka{Brokers: brokers}
	const topic, group = "order.events", "dlq-failure"

	p, err := kafkax.NewProducer(kc, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	for _, r := range []*kgo.Record{
		{Topic: topic, Key: []byte("malformed"), Value: []byte{0xff}},
		{Topic: topic, Key: []byte("o1"), Value: paidEnvelope(t, "o1", "s1")},
	} {
		if err := p.PublishRecord(context.Background(), r); err != nil {
			t.Fatal(err)
		}
	}

	apply := &creditApplier{}
	h := settlement.New(apply, slog.New(slog.NewTextHandler(io.Discard, nil)))
	run := func(dlq consume.DLQProducer) error {
		client, err := kafkax.NewConsumerClient(kc, group, []string{topic},
			kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()), kgo.AllowAutoTopicCreation())
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		c := consume.NewConsumer(client, h.Handle, dlq, consume.Options{
			DLQTopic: "order.events.payment-settlement.dlq", MaxAttempts: 2, BaseBackoff: time.Millisecond,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return c.Run(ctx)
	}

	down := &recordingDLQ{fail: true}
	if err := run(down); err == nil {
		t.Fatal("consumer returned nil although the DLQ produce failed")
	}
	if got := apply.got(); len(got) != 0 {
		t.Fatalf("a later record was applied while an earlier one was stuck: %v", got)
	}

	// Restart in the same group with a healthy DLQ: the malformed record comes back.
	up := &recordingDLQ{}
	ctx, cancel := context.WithCancel(context.Background())
	client, err := kafkax.NewConsumerClient(kc, group, []string{topic},
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()), kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	c := consume.NewConsumer(client, h.Handle, up, consume.Options{DLQTopic: "dlq", MaxAttempts: 2, BaseBackoff: time.Millisecond})
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	deadline := time.Now().Add(30 * time.Second)
	for (len(up.parked()) < 1 || len(apply.got()) < 1) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	if got := up.parked(); len(got) != 1 || got[0] != "malformed" {
		t.Fatalf("dead-lettered after restart = %v, want [malformed]", got)
	}
	if got := apply.got(); len(got) != 1 || got[0] != "o1" {
		t.Fatalf("credited after restart = %v, want [o1]", got)
	}
}
