package events

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
)

const runWait = 30 * time.Second

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(runWait)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met in time")
}

func produceTo(t *testing.T, brokers, topic string, values ...[]byte) {
	t.Helper()
	p, err := kafkax.NewProducer(config.Kafka{Brokers: brokers}, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatalf("producer: %v", err)
	}
	defer p.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, v := range values {
		if err := p.Publish(ctx, topic, []byte("k"), v); err != nil {
			t.Fatalf("produce: %v", err)
		}
	}
}

// startRunner runs handle over topic in group and returns Run's result channel.
func startRunner(ctx context.Context, t *testing.T, brokers, group, topic string, handle HandlerFunc, dlq *kafkax.Producer) <-chan error {
	t.Helper()
	client, err := kafkax.NewConsumerClient(config.Kafka{Brokers: brokers}, group, []string{topic})
	if err != nil {
		t.Fatalf("consumer client: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		err := NewRunner(client, handle, dlq, topic+".dlq", nil).Run(ctx)
		client.Close() // leave the group so a restart in it is assigned the partition at once
		done <- err
	}()
	return done
}

// A poison record is parked on the DLQ, the offsets of the poison and of the good
// record behind it are both committed, and the good record still notifies.
func TestRun_MalformedRecord_RoutedToDLQAndCommitted(t *testing.T) {
	brokers := testkit.Redpanda(t)
	notif := &fakeNotif{}
	subs := &fakeSubs{byKey: map[string][]*notificationv1.AlertSubscription{
		subKey("listing_1", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP): {
			sub("user_a", "listing_1", notificationv1.AlertType_ALERT_TYPE_PRICE_DROP),
		},
	}}
	c := newConsumer(notif, subs)

	// Seed the last-seen price so the "good" record (price 80) reads as a drop.
	if err := c.HandleRaw(context.Background(), priceEnvelope(t, "evt_seed", "listing_1", 100)); err != nil {
		t.Fatalf("HandleRaw seed: %v", err)
	}

	poison := []byte("\xff\xff not a protobuf")
	good := priceEnvelope(t, "evt_ok", "listing_1", 80)
	produceTo(t, brokers, "listing.events", poison, good)

	dlq, err := kafkax.NewProducer(config.Kafka{Brokers: brokers}, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatal(err)
	}
	defer dlq.Close()

	ctx, cancel := context.WithTimeout(context.Background(), runWait)
	defer cancel()
	done := startRunner(ctx, t, brokers, "g-run", "listing.events", c.HandleRaw, dlq)
	waitFor(t, func() bool { return notif.count() == 1 })
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Exactly the poison record is on the DLQ topic.
	reader, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.ConsumeTopics("listing.events.dlq"),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	pctx, pcancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer pcancel()
	var parked [][]byte
	reader.PollFetches(pctx).EachRecord(func(r *kgo.Record) { parked = append(parked, r.Value) })
	if len(parked) != 1 || string(parked[0]) != string(poison) {
		t.Fatalf("want exactly the poison record on listing.events.dlq, got %d records", len(parked))
	}

	// Both offsets were committed: a restart in the same group sees only a new record.
	sentinel := priceEnvelope(t, "evt_after", "listing_1", 70)
	produceTo(t, brokers, "listing.events", sentinel)
	var mu sync.Mutex
	var seen [][]byte
	ctx2, cancel2 := context.WithTimeout(context.Background(), runWait)
	defer cancel2()
	done2 := startRunner(ctx2, t, brokers, "g-run", "listing.events", func(_ context.Context, v []byte) error {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, v)
		return nil
	}, dlq)
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(seen) >= 1 })
	cancel2()
	if err := <-done2; err != nil {
		t.Fatalf("second Run: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || string(seen[0]) != string(sentinel) {
		t.Fatalf("restart redelivered committed records: got %d records", len(seen))
	}
}

// Fetch errors (a broker that cannot be reached) must not stop the loop while its
// own context is alive; cancelling the context ends it cleanly.
func TestRun_TransientFetchErrorsDoNotStopTheLoop(t *testing.T) {
	client, err := kafkax.NewConsumerClient(config.Kafka{Brokers: "127.0.0.1:1"}, "g-unreachable", []string{"listing.events"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- NewRunner(client, func(context.Context, []byte) error { return nil }, nil, "listing.events.dlq", nil).Run(ctx)
	}()

	select {
	case err := <-done:
		t.Fatalf("loop stopped on a transient error: %v", err)
	case <-time.After(2 * time.Second):
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("want a clean stop on shutdown, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("loop did not stop on cancel")
	}
}
