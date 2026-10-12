package events_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-analytics/internal/events"
	"github.com/buidangphuc/team-analytics/internal/store"
)

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

type recordingWriter struct {
	mu   sync.Mutex
	rows []*store.EngagementFactRecord
}

func (w *recordingWriter) WriteEngagementFacts(_ context.Context, b []*store.EngagementFactRecord) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.rows = append(w.rows, b...)
	return nil
}

// Scenario: team-analytics consumer does not commit when the DLQ produce fails.
// An undecodable engagement record must be dead-lettered before its offset is
// committed. The DLQ topic does not exist and the client may not create it, so
// the DLQ produce fails: the offset must stay uncommitted and the record must
// be redelivered to the next consumer of the group.
func TestConsumerDoesNotCommitOnDLQFailure(t *testing.T) {
	brokers := testkit.Redpanda(t)
	kc := sdkconfig.Kafka{Brokers: brokers}
	const (
		topic = "engagement.events"
		dlq   = "engagement.events.analytics.dlq"
		group = "team-analytics.engagement.test"
	)
	poison := []byte("not an envelope \xff\xff")

	prod, err := kafkax.NewProducer(kc, kgo.AllowAutoTopicCreation())
	require.NoError(t, err)
	require.NoError(t, prod.Publish(context.Background(), topic, []byte("k0"), poison))
	prod.Close()

	newClient := func() *kgo.Client {
		c, cerr := kafkax.NewConsumerClient(kc, group, []string{topic},
			kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
			kgo.RecordDeliveryTimeout(2*time.Second),
			kgo.UnknownTopicRetries(0))
		require.NoError(t, cerr)
		return c
	}

	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	w := &recordingWriter{}
	c1 := events.NewEngagementConsumer(newClient(), dlq)
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = c1.Run(runCtx, w, logger)
	}()
	require.Eventually(t, func() bool {
		return strings.Contains(logs.String(), "engagement batch failed")
	}, 60*time.Second, 200*time.Millisecond, "the DLQ produce was never attempted")
	cancel()
	<-done
	c1.Close()
	assert.Empty(t, w.rows)

	// A restarted consumer of the same group gets the record again.
	c2 := newClient()
	defer c2.Close()
	pollCtx, pollCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer pollCancel()
	var got *kgo.Record
	for got == nil && pollCtx.Err() == nil {
		it := c2.PollFetches(pollCtx).RecordIter()
		for !it.Done() {
			got = it.Next()
		}
	}
	require.NotNil(t, got, "record was not redelivered after restart")
	assert.Equal(t, poison, got.Value)
}
