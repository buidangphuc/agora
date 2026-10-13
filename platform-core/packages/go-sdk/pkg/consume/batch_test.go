package consume

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

type batchDLQ struct {
	mu   sync.Mutex
	recs []*kgo.Record
}

func (d *batchDLQ) PublishRecord(_ context.Context, r *kgo.Record) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.recs = append(d.recs, r)
	return nil
}

func scriptedFetch(topic string, values ...string) kgo.Fetches {
	recs := make([]*kgo.Record, len(values))
	for i, v := range values {
		recs[i] = &kgo.Record{Topic: topic, Partition: 0, Offset: int64(i), Key: []byte("k"), Value: []byte(v),
			Headers: []kgo.RecordHeader{{Key: "x-h", Value: []byte("hv")}}}
	}
	return kgo.Fetches{{Topics: []kgo.FetchTopic{{Topic: topic, Partitions: []kgo.FetchPartition{{Partition: 0, Records: recs}}}}}}
}

// newScriptedBatch returns a consumer that serves one fetch, then blocks until
// ctx ends. commits counts CommitUncommittedOffsets calls.
func newScriptedBatch(h BatchHandler, dlq DLQProducer, commits *chan struct{}) *BatchConsumer {
	served := false
	c := NewBatchConsumer(nil, h, dlq, BatchOptions{DLQTopic: "orders.dlq", BaseBackoff: time.Millisecond, MaxBackoff: 5 * time.Millisecond})
	c.poll = func(ctx context.Context) kgo.Fetches {
		if !served {
			served = true
			return scriptedFetch("orders", "v0", "v1")
		}
		<-ctx.Done()
		return nil
	}
	c.commit = func(context.Context) error { *commits <- struct{}{}; return nil }
	return c
}

func runBatch(t *testing.T, c *BatchConsumer, until func() bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	deadline := time.After(10 * time.Second)
	for !until() {
		select {
		case <-deadline:
			cancel()
			t.Fatal("condition not reached")
		case <-time.After(2 * time.Millisecond):
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestBatchRetriesWithoutCommit(t *testing.T) {
	commits := make(chan struct{}, 10)
	var mu sync.Mutex
	var seen [][]string
	h := func(_ context.Context, rs []Record) error {
		mu.Lock()
		defer mu.Unlock()
		var vs []string
		for _, r := range rs {
			vs = append(vs, string(r.Value))
		}
		seen = append(seen, vs)
		if len(seen) == 1 {
			if len(commits) != 0 {
				t.Error("committed before the handler succeeded")
			}
			return errors.New("transient")
		}
		return nil
	}
	c := newScriptedBatch(h, &batchDLQ{}, &commits)
	runBatch(t, c, func() bool { return len(commits) == 1 })

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 || len(seen[0]) != 2 || seen[0][0] != "v0" || seen[1][1] != "v1" || seen[1][0] != "v0" {
		t.Fatalf("handler saw %v, want the same two records twice", seen)
	}
	if len(commits) != 1 {
		t.Fatalf("commits = %d, want exactly 1 after success", len(commits))
	}
}

func TestBatchPermanentGoesToDLQ(t *testing.T) {
	commits := make(chan struct{}, 10)
	dlq := &batchDLQ{}
	calls := 0
	h := func(context.Context, []Record) error { calls++; return Permanent(errors.New("poison")) }
	c := newScriptedBatch(h, dlq, &commits)
	runBatch(t, c, func() bool { return len(commits) == 1 })

	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1 (permanent is not retried)", calls)
	}
	dlq.mu.Lock()
	defer dlq.mu.Unlock()
	if len(dlq.recs) != 2 {
		t.Fatalf("dlq records = %d, want 2", len(dlq.recs))
	}
	for i, r := range dlq.recs {
		hs := map[string]string{}
		for _, h := range r.Headers {
			hs[h.Key] = string(h.Value)
		}
		if r.Topic != "orders.dlq" || hs[HeaderError] != "poison" || hs[HeaderSourceTopic] != "orders" || hs["x-h"] != "hv" {
			t.Fatalf("dlq record %d = topic %q headers %v", i, r.Topic, hs)
		}
	}
}
