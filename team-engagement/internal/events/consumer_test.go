package events_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-engagement/internal/events"
)

type failingDLQ struct{}

func (failingDLQ) PublishRecord(context.Context, *kgo.Record) error {
	return errors.New("dlq unreachable")
}

type noopFeed struct{}

func (noopFeed) UpsertSellerListing(context.Context, string, string, time.Time) error { return nil }
func (noopFeed) RemoveSellerListing(context.Context, string) error                    { return nil }

// A record that fails permanently while the DLQ produce also fails must stop the
// consumer with an error and stay uncommitted: after a restart in the same group
// it is delivered again and, with a working DLQ, parked on <topic>.dlq.
func TestConsumerDoesNotCommitOnDLQFailure(t *testing.T) {
	brokers := testkit.Redpanda(t)
	kc := sdkconfig.Kafka{Brokers: brokers}
	const (
		topic = "listing.events"
		dlq   = "listing.events.dlq"
		group = "team-engagement-feed-test"
	)
	logger := slog.New(slog.DiscardHandler)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	producer, err := kafkax.NewProducer(kc, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	garbage := []byte("\xff\xff not proto")
	if err := producer.Publish(ctx, topic, []byte("k1"), garbage); err != nil {
		t.Fatal(err)
	}

	// First run: the DLQ is down, so Run must return the error.
	c1, err := events.NewListingClient(kc, group, topic, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatal(err)
	}
	runErr := events.NewListingConsumer(c1, noopFeed{}, failingDLQ{}, topic, logger).Run(ctx)
	c1.Close()
	if runErr == nil {
		t.Fatal("Run returned nil although the record could be neither handled nor dead-lettered")
	}

	// Restart in the same group with a working DLQ: the record comes again.
	c2, err := events.NewListingClient(kc, group, topic, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- events.NewListingConsumer(c2, noopFeed{}, producer, topic, logger).Run(runCtx) }()

	dlqReader, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.ConsumeTopics(dlq),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()), kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatal(err)
	}
	defer dlqReader.Close()
	var got *kgo.Record
	for got == nil && ctx.Err() == nil {
		pollCtx, c := context.WithTimeout(ctx, 3*time.Second)
		iter := dlqReader.PollFetches(pollCtx).RecordIter()
		for !iter.Done() {
			got = iter.Next()
		}
		c()
	}
	if got == nil {
		t.Fatal("the record was not redelivered to the dead-letter topic after the restart")
	}
	if string(got.Value) != string(garbage) || string(got.Key) != "k1" {
		t.Fatalf("dlq record = %q/%q, want the original", got.Key, got.Value)
	}
	stop()
	if err := <-done; err != nil {
		t.Fatalf("second Run = %v, want nil on shutdown", err)
	}
}
