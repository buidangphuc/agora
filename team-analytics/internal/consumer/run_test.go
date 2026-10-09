package consumer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-analytics/generated/platform/order/v1"
	"github.com/buidangphuc/team-analytics/internal/warehouse/fake"
)

// scriptedClient serves canned fetches, then cancels the run context. It
// records how many commits had happened by the time each poll was made.
type scriptedClient struct {
	mu            sync.Mutex
	script        []kgo.Fetches
	cancel        context.CancelFunc
	commits       int
	commitsAtPoll []int
}

func (s *scriptedClient) PollFetches(context.Context) kgo.Fetches {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commitsAtPoll = append(s.commitsAtPoll, s.commits)
	if len(s.script) == 0 {
		s.cancel()
		return nil
	}
	f := s.script[0]
	s.script = s.script[1:]
	return f
}

func (s *scriptedClient) CommitUncommittedOffsets(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commits++
	return nil
}

func (s *scriptedClient) Close() {}

func fetchOf(values ...[]byte) kgo.Fetches {
	recs := make([]*kgo.Record, len(values))
	for i, v := range values {
		recs[i] = &kgo.Record{Value: v}
	}
	return kgo.Fetches{{Topics: []kgo.FetchTopic{{
		Topic:      "t",
		Partitions: []kgo.FetchPartition{{Partition: 0, Records: recs}},
	}}}}
}

func envelope(t *testing.T, typ string, payload proto.Message) []byte {
	t.Helper()
	body, err := proto.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	v, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: "e", Type: typ, Payload: body})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func trackingValue(t *testing.T) []byte {
	return envelope(t, TrackingEventType, &analyticsv1.TrackingEvent{ListingId: "l1"})
}

func orderPaidValue(t *testing.T) []byte {
	return envelope(t, OrderPaidEventType, &orderv1.OrderPaidEvent{
		OrderId: "o1", BuyerId: "b1", PaidAt: timestamppb.Now(),
		Items: []*orderv1.OrderLineItemFact{{ListingId: "l1", SellerId: "s1", Quantity: 1}},
	})
}

func runScripted(t *testing.T, w *fake.Writer, batchSize int, script ...kgo.Fetches) *scriptedClient {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sc := &scriptedClient{script: script, cancel: cancel}
	c := &Consumer{client: sc}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, w, batchSize, 0, logger) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}
	return sc
}

// A fetch holding only a skipped envelope (OrderShipped on order.events) must
// still advance the group offset.
func TestRunCommitsAfterSkipOnlyFetch(t *testing.T) {
	shipped := envelope(t, "platform.order.v1.OrderShipped", &orderv1.OrderShipped{OrderId: "o1"})
	sc := runScripted(t, fake.New(), 10, fetchOf(shipped))
	if sc.commitsAtPoll[1] != 1 {
		t.Fatalf("commits after skip-only fetch = %d, want 1", sc.commitsAtPoll[1])
	}
}

// Flushing the tracking batch must not commit offsets of an order fact that is
// still buffered; the commit happens once the order batch is written too.
func TestRunNoCommitWhileOtherBatcherBuffered(t *testing.T) {
	w := fake.New()
	sc := runScripted(t, w, 2,
		fetchOf(orderPaidValue(t), trackingValue(t), trackingValue(t)), // tracking flushes, 1 order fact buffered
		fetchOf(orderPaidValue(t)),                                     // order batch fills and flushes
	)
	if len(w.Rows()) != 2 {
		t.Fatalf("tracking rows = %d, want 2", len(w.Rows()))
	}
	if sc.commitsAtPoll[1] != 0 {
		t.Fatalf("committed %d time(s) while an order fact was still buffered", sc.commitsAtPoll[1])
	}
	if sc.commitsAtPoll[2] != 1 {
		t.Fatalf("commits after both batches flushed = %d, want 1", sc.commitsAtPoll[2])
	}
}

// A failed write keeps records buffered and must never commit, including on
// the shutdown flush.
func TestRunNoCommitOnFailedWrite(t *testing.T) {
	w := fake.New()
	w.FailWith = errors.New("warehouse down")
	sc := runScripted(t, w, 1, fetchOf(trackingValue(t)))
	if sc.commits != 0 {
		t.Fatalf("commits = %d after failed write, want 0", sc.commits)
	}
}
