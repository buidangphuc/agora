package events_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/store"
)

// AD1: the consume loop commits only after apply/DLQ, and DLQs poison records. A
// poison record goes to the dead-letter topic and the good record behind it is
// applied; both offsets are committed, so a restart in the same group delivers
// nothing again.
func TestPaymentConsumer_Run_PoisonRecordGoesToDLQ_ThenCommits(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	brokers := testkit.Redpanda(t)
	cfg := sdkconfig.Kafka{Brokers: brokers}
	pool := orderDB(t)
	orders := store.NewPostgresOrderRepository(pool)
	handler := events.NewPaymentConsumer(store.NewPostgresTransactor(pool, orders), nil)
	o := pendingOrder(t, orders, "")

	producer, err := kafkax.NewProducer(cfg, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	if err := producer.Publish(ctx, paymentTopic, []byte("ord_x"), []byte("not a valid envelope")); err != nil { // poison -> DLQ
		t.Fatal(err)
	}
	good := settledRecord(t, "evt_ok", o.ID)
	if err := producer.Publish(ctx, paymentTopic, []byte(o.ID), good.Value); err != nil { // applied
		t.Fatal(err)
	}

	const group = "team-order.payment.poison-test"
	opts := consume.Options{DLQTopic: paymentDLQTopic, MaxAttempts: 2, BaseBackoff: time.Millisecond}
	client, err := kafkax.NewConsumerClient(cfg, group, []string{paymentTopic})
	if err != nil {
		t.Fatal(err)
	}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	loop, err := consume.NewConsumer(client, handler.Handle, producer, opts)
	if err != nil {
		t.Fatal(err)
	}
	go func() { done <- loop.Run(runCtx) }()

	deadline := time.Now().Add(90 * time.Second)
	for orderStatus(t, orders, o.ID) != store.OrderStatusPaid {
		if time.Now().After(deadline) {
			t.Fatal("the good record behind the poison one was never applied")
		}
		time.Sleep(50 * time.Millisecond)
	}
	stop()
	if err := <-done; err != nil {
		t.Fatalf("consumer stopped with %v", err)
	}
	client.Close()

	dlqReader, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.ConsumeTopics(paymentDLQTopic), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer dlqReader.Close()
	pollCtx, stopPoll := context.WithTimeout(ctx, 60*time.Second)
	defer stopPoll()
	var dead []*kgo.Record
	for len(dead) == 0 && pollCtx.Err() == nil {
		dlqReader.PollFetches(pollCtx).EachRecord(func(r *kgo.Record) { dead = append(dead, r) })
	}
	if len(dead) != 1 || string(dead[0].Value) != "not a valid envelope" {
		t.Fatalf("expected exactly the poison record in the DLQ, got %d record(s)", len(dead))
	}

	// Both records were committed: a restarted consumer in the group sees none.
	again, err := kafkax.NewConsumerClient(cfg, group, []string{paymentTopic})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	var redelivered atomic.Int32
	quiet, stopQuiet := context.WithTimeout(ctx, 8*time.Second)
	defer stopQuiet()
	check, err := consume.NewConsumer(again, func(context.Context, consume.Record) error {
		redelivered.Add(1)
		return nil
	}, producer, opts)
	if err != nil {
		t.Fatal(err)
	}
	_ = check.Run(quiet)
	if n := redelivered.Load(); n != 0 {
		t.Fatalf("%d record(s) redelivered after both were handled or dead-lettered", n)
	}
}
