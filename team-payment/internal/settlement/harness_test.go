package settlement_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	sdkconsume "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"

	"github.com/buidangphuc/team-payment/internal/pgtest"
	"github.com/buidangphuc/team-payment/internal/settlement"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Run(m)) }

// record is one message the tests produce to order.events.
type record struct {
	Key   string
	Value []byte
}

// fakeReader is the set of records a run produces, plus what the consumer group
// committed: the first N records for a committed offset N.
type fakeReader struct {
	records []record
	client  atomic.Pointer[kgo.Client]
	topic   string
}

func (r *fakeReader) commits() []string {
	c := r.client.Load()
	if c == nil {
		return nil
	}
	off := c.CommittedOffsets()[r.topic][0].Offset
	var out []string
	for i := int64(0); i < off && i < int64(len(r.records)); i++ {
		out = append(out, r.records[i].Key)
	}
	return out
}

// fakeDLQ records the dead-letter records the consumer produced.
type fakeDLQ struct {
	mu    sync.Mutex
	topic string
	keys  []string
}

func (d *fakeDLQ) PublishRecord(_ context.Context, rec *kgo.Record) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.topic = rec.Topic
	d.keys = append(d.keys, string(rec.Key))
	return nil
}

func (d *fakeDLQ) parked() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.keys...)
}

func committedN(r *fakeReader, n int) func() bool {
	return func() bool { return len(r.commits()) >= n }
}

// slow makes any retry visible: a retried record would block the test for an hour.
var slow = sdkconsume.Options{BaseBackoff: time.Hour}

var quiet = slog.New(slog.DiscardHandler)

var topicSeq atomic.Int32

// run produces the reader's records to a fresh topic, drives the SDK consume loop over
// the shared Redpanda until cond holds (or fails after 40s), then stops it.
func run(t *testing.T, apply settlement.Applier, reader *fakeReader, dlq *fakeDLQ, opts sdkconsume.Options, cond func() bool) {
	t.Helper()
	brokers := pgtest.Brokers(t)
	kc := sdkconfig.Kafka{Brokers: brokers}
	n := topicSeq.Add(1)
	reader.topic = fmt.Sprintf("order.events.t%d.%d", os.Getpid(), n)
	group := "g-" + reader.topic

	p, err := kafkax.NewProducer(kc, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range reader.records {
		if err := p.PublishRecord(context.Background(), &kgo.Record{Topic: reader.topic, Key: []byte(r.Key), Value: r.Value}); err != nil {
			t.Fatal(err)
		}
	}
	p.Close()

	client, err := kafkax.NewConsumerClient(kc, group, []string{reader.topic},
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()), kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	reader.client.Store(client)
	if opts.DLQTopic == "" {
		opts.DLQTopic = settlement.DefaultDLQTopic
	}
	c, err := sdkconsume.NewConsumer(client, settlement.New(apply, quiet).Handle, dlq, opts)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	deadline := time.Now().Add(40 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatalf("condition not met; committed=%v dlq=%v", reader.commits(), dlq.parked())
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v", err)
	}
}
