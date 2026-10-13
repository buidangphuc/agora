package events_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-notification/internal/events"
)

const waitFor = 30 * time.Second

type recordingDLQ struct {
	mu   sync.Mutex
	err  error
	recs []*kgo.Record
}

func (d *recordingDLQ) PublishRecord(_ context.Context, r *kgo.Record) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return d.err
	}
	d.recs = append(d.recs, r)
	return nil
}

func (d *recordingDLQ) records() []*kgo.Record {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]*kgo.Record(nil), d.recs...)
}

func produce(t *testing.T, brokers, topic, value string) {
	t.Helper()
	testkit.Produce(t, brokers, topic, "k", []byte(value))
}

// runner starts events.NewRunner in group over topic with a chat consumer's
// handler and returns the channel that receives Run's result.
func runner(ctx context.Context, t *testing.T, brokers, group string, dlq *recordingDLQ) <-chan error {
	t.Helper()
	client, err := kafkax.NewConsumerClient(config.Kafka{Brokers: brokers}, group, []string{"chat.events"})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	h := events.NewChatConsumer(nil, nil, nil)
	c, err := events.NewRunner(client, h.HandleRaw, dlq, "chat.events.dlq", nil)
	if err != nil {
		t.Fatalf("runner: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		err := c.Run(ctx)
		client.Close() // leave the group so a restart in it is assigned the partition at once
		done <- err
	}()
	return done
}

func TestConsumerDoesNotCommitOnDLQFailure(t *testing.T) {
	brokers := testkit.Redpanda(t)
	produce(t, brokers, "chat.events", "\xff\xff not a protobuf")

	// First run: the record is poison (a permanent error) and the DLQ produce
	// fails. The consumer must stop with an error and must not commit.
	failing := &recordingDLQ{err: errors.New("dlq down")}
	ctx1, cancel1 := context.WithTimeout(context.Background(), waitFor)
	defer cancel1()
	select {
	case err := <-runner(ctx1, t, brokers, "g-dlq", failing):
		if err == nil {
			t.Fatal("Run returned nil although the DLQ produce failed")
		}
	case <-time.After(waitFor):
		t.Fatal("consumer did not stop on a failed DLQ produce")
	}

	// Restart in the same group: the same record is delivered again and, with
	// a working DLQ, is parked there.
	healthy := &recordingDLQ{}
	ctx2, cancel2 := context.WithTimeout(context.Background(), waitFor)
	defer cancel2()
	done := runner(ctx2, t, brokers, "g-dlq", healthy)
	testkit.Eventually(t, waitFor, func() bool { return len(healthy.records()) > 0 })
	cancel2()
	if err := <-done; err != nil {
		t.Fatalf("second Run: %v", err)
	}
	recs := healthy.records()
	if len(recs) != 1 || recs[0].Topic != "chat.events.dlq" {
		t.Fatalf("after restart the poison record was not redelivered to the DLQ: %v", recs)
	}
}
