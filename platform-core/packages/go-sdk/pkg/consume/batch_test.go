package consume

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
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

func scriptedBatch(t *testing.T, h BatchHandler, dlq DLQProducer) (*BatchConsumer, *fakeClient) {
	t.Helper()
	fc := newFakeClient(fetchOf("orders", span{
		partition: 0, from: 0, count: 2,
		headers: []kgo.RecordHeader{{Key: "x-h", Value: []byte("hv")}},
	}))
	c, err := newBatchConsumer(fc, "g", h, dlq, fastBatchOpts())
	if err != nil {
		t.Fatal(err)
	}
	return c, fc
}

func TestBatchRetriesWithoutCommit(t *testing.T) {
	var fcRef *fakeClient
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
			if _, commits := fcRef.snapshot(); len(commits) != 0 {
				t.Error("committed before the handler succeeded")
			}
			return errors.New("transient")
		}
		return nil
	}
	c, fc := scriptedBatch(t, h, &recordingDLQ{})
	fcRef = fc
	if err := runUntil(t, c.Run, func() { fc.waitForEvent(t, "commit", 1) }); err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 || len(seen[0]) != 2 || seen[0][0] != "p0-o0" || seen[1][1] != "p0-o1" || seen[1][0] != "p0-o0" {
		t.Fatalf("handler saw %v, want the same two records twice", seen)
	}
	if _, commits := fc.snapshot(); len(commits) != 1 {
		t.Fatalf("commits = %d, want exactly 1 after success", len(commits))
	}
}

func TestBatchPermanentGoesToDLQ(t *testing.T) {
	dlq := &recordingDLQ{}
	calls := 0
	h := func(context.Context, []Record) error { calls++; return Permanent(errors.New("poison")) }
	c, fc := scriptedBatch(t, h, dlq)
	if err := runUntil(t, c.Run, func() { fc.waitForEvent(t, "commit", 1) }); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1 (permanent is not retried)", calls)
	}
	recs := dlq.records()
	if len(recs) != 2 {
		t.Fatalf("dlq records = %d, want 2", len(recs))
	}
	for i, r := range recs {
		hs := map[string]string{}
		for _, h := range r.Headers {
			hs[h.Key] = string(h.Value)
		}
		if r.Topic != "orders.dlq" || hs[kafkax.HeaderDLQError] != "poison" || hs[kafkax.HeaderDLQSourceTopic] != "orders" || hs["x-h"] != "hv" {
			t.Fatalf("dlq record %d = topic %q headers %v", i, r.Topic, hs)
		}
	}
}
