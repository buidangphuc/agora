package consume

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"
)

func fastOpts() Options {
	return Options{DLQTopic: "orders.dlq", MaxAttempts: 3, BaseBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond}
}

func fastBatchOpts() BatchOptions {
	return BatchOptions{DLQTopic: "orders.dlq", MaxAttempts: 3, BaseBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond}
}

// commitMap flattens commits into partition -> committed offsets in order.
func commitOffsets(commits [][]*kgo.Record) map[int32][]int64 {
	m := map[int32][]int64{}
	for _, c := range commits {
		for _, r := range c {
			m[r.Partition] = append(m[r.Partition], r.Offset)
		}
	}
	return m
}

func TestRevokeMidFetchDoesNotRewindOrDoubleCommit(t *testing.T) {
	// Fetch 1: two partitions. Fetch 2: the same partitions after the revoke and
	// re-assign, continuing at the committed offsets.
	fc := newFakeClient(
		fetchOf("orders", span{partition: 0, from: 10, count: 3}, span{partition: 1, from: 5, count: 2}),
		fetchOf("orders", span{partition: 0, from: 13, count: 1}),
	)
	handled := 0
	c, err := newConsumer(fc, "g", func(context.Context, Record) error {
		handled++
		if handled == 2 {
			fc.requestRevoke() // the broker wants the partitions mid-fetch
		}
		return nil
	}, nopDLQ{}, fastOpts())
	if err != nil {
		t.Fatal(err)
	}
	if err := runUntil(t, c.Run, func() { fc.waitForEvent(t, "commit", 2) }); err != nil {
		t.Fatalf("Run: %v", err)
	}

	events, commits := fc.snapshot()
	// Per fetch: poll, one commit, then allow; the revoke lands only after the
	// first commit, so it never sees uncommitted work and never races it.
	want := []string{"poll", "commit", "revoke", "allow", "poll", "commit", "allow"}
	if got := strings.Join(events[:len(want)], ","); got != strings.Join(want, ",") {
		t.Fatalf("events = %v, want prefix %v", events, want)
	}
	if len(commits) != 2 {
		t.Fatalf("commit calls = %d, want 1 per fetch (2)", len(commits))
	}
	if len(commits[0]) != 2 {
		t.Fatalf("first commit covers %d partitions, want 2", len(commits[0]))
	}
	got := commitOffsets(commits)
	if p0 := got[0]; len(p0) != 2 || p0[0] != 12 || p0[1] != 13 {
		t.Fatalf("partition 0 commits = %v, want [12 13] (last record of each fetch, never a rewind)", p0)
	}
	if p1 := got[1]; len(p1) != 1 || p1[0] != 6 {
		t.Fatalf("partition 1 commits = %v, want [6]", p1)
	}
}

func TestFailedRecordIsNeverCommittedAndRebalanceIsAllowed(t *testing.T) {
	fc := newFakeClient(fetchOf("orders", span{partition: 0, from: 0, count: 3}))
	dlqErr := errors.New("dlq down")
	c, err := newConsumer(fc, "g", func(_ context.Context, r Record) error {
		if r.Offset == 1 {
			return Permanent(errors.New("poison"))
		}
		return nil
	}, &recordingDLQ{err: dlqErr}, fastOpts())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Run(context.Background()); !errors.Is(err, dlqErr) {
		t.Fatalf("Run = %v, want the dlq error", err)
	}
	events, commits := fc.snapshot()
	if len(commits) != 1 || len(commits[0]) != 1 || commits[0][0].Offset != 0 {
		t.Fatalf("commits = %v, want only offset 0 (handled before the failure)", commitOffsets(commits))
	}
	if strings.Join(events, ",") != "poll,commit,allow,allow" {
		t.Fatalf("events = %v, want rebalances allowed on the error path", events)
	}
}

func TestDLQFailureCommitsNothing(t *testing.T) {
	fc := newFakeClient(fetchOf("orders", span{partition: 0, from: 0, count: 1}))
	c, _ := newConsumer(fc, "g", func(context.Context, Record) error { return Permanent(errors.New("bad")) },
		&recordingDLQ{err: errors.New("down")}, fastOpts())
	if err := c.Run(context.Background()); err == nil {
		t.Fatal("Run returned nil with the DLQ down")
	}
	if _, commits := fc.snapshot(); len(commits) != 0 {
		t.Fatalf("commits = %v, want none", commits)
	}
}

func installTracing(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	prevTP, prevProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
		_ = tp.Shutdown(context.Background())
	})
	return exp
}

// producerHeaders returns the headers kafkax.Producer would stamp for a request.
func producerHeaders(t *testing.T, traceparent, requestID string) []kgo.RecordHeader {
	t.Helper()
	return []kgo.RecordHeader{
		{Key: kafkax.HeaderTraceparent, Value: []byte(traceparent)},
		{Key: kafkax.HeaderRequestID, Value: []byte(requestID)},
	}
}

const (
	tp1 = "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	tp2 = "00-1af7651916cd43dd8448eb211c80319c-c7ad6b7169203331-01"
)

func TestTraceAndRequestIDReachHandlerContext(t *testing.T) {
	installTracing(t)
	fc := newFakeClient(fetchOf("orders", span{partition: 0, from: 0, count: 1, headers: producerHeaders(t, tp1, "req-1")}))
	var mu sync.Mutex
	var gotTrace, gotReq string
	dlq := &recordingDLQ{}
	c, _ := newConsumer(fc, "g", func(ctx context.Context, _ Record) error {
		mu.Lock()
		defer mu.Unlock()
		gotTrace = trace.SpanContextFromContext(ctx).TraceID().String()
		gotReq = reqctx.RequestID(ctx)
		return Permanent(errors.New("park it"))
	}, dlq, fastOpts())
	_ = runUntil(t, c.Run, func() { fc.waitForEvent(t, "commit", 1) })

	mu.Lock()
	defer mu.Unlock()
	if gotTrace != "0af7651916cd43dd8448eb211c80319c" || gotReq != "req-1" {
		t.Fatalf("handler ctx trace = %q request id = %q", gotTrace, gotReq)
	}
	if n := len(dlq.records()); n != 1 {
		t.Fatalf("dlq records = %d, want 1", n)
	}
}

func TestBatchContextLinksRecordSpans(t *testing.T) {
	exp := installTracing(t)
	fc := newFakeClient(fetchOf("orders",
		span{partition: 0, from: 0, count: 1, headers: producerHeaders(t, tp1, "same")},
		span{partition: 1, from: 0, count: 1, headers: producerHeaders(t, tp2, "same")}))
	var mu sync.Mutex
	var gotReq string
	c, _ := newBatchConsumer(fc, "g", func(ctx context.Context, _ []Record) error {
		mu.Lock()
		defer mu.Unlock()
		gotReq = reqctx.RequestID(ctx)
		return nil
	}, nopDLQ{}, fastBatchOpts())
	_ = runUntil(t, c.Run, func() { fc.waitForEvent(t, "commit", 1) })

	mu.Lock()
	defer mu.Unlock()
	if gotReq != "same" {
		t.Fatalf("request id = %q, want the shared one", gotReq)
	}
	var found bool
	for _, s := range exp.GetSpans() {
		if s.Name == "consume batch" {
			found = true
			if len(s.Links) != 2 {
				t.Fatalf("batch span links = %d, want 2", len(s.Links))
			}
		}
	}
	if !found {
		t.Fatal("no consume batch span exported")
	}
}

func TestRecordCarriesPartitionTimestampAndDuplicateHeaders(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	rec := &kgo.Record{
		Topic: "orders", Partition: 3, Offset: 42, Timestamp: ts, Key: []byte("k"), Value: []byte("v"),
		Headers: []kgo.RecordHeader{{Key: "h", Value: []byte("1")}, {Key: "h", Value: []byte("2")}},
	}
	fc := newFakeClient(kgo.Fetches{{Topics: []kgo.FetchTopic{{Topic: "orders", Partitions: []kgo.FetchPartition{{Partition: 3, Records: []*kgo.Record{rec}}}}}}})
	got := make(chan Record, 1)
	dlq := &recordingDLQ{}
	c, _ := newConsumer(fc, "billing", func(_ context.Context, r Record) error {
		got <- r
		return Permanent(errors.New("poison"))
	}, dlq, fastOpts())
	_ = runUntil(t, c.Run, func() { fc.waitForEvent(t, "commit", 1) })

	r := <-got
	if r.Partition != 3 || r.Offset != 42 || !r.Timestamp.Equal(ts) {
		t.Fatalf("record = %+v", r)
	}
	if len(r.Headers) != 2 || string(r.Headers[0].Value) != "1" || string(r.Headers[1].Value) != "2" {
		t.Fatalf("headers = %+v, want both duplicates in order", r.Headers)
	}
	if v, ok := r.Header("h"); !ok || string(v) != "1" {
		t.Fatalf("Header(h) = %q %v", v, ok)
	}
	d := dlq.records()[0]
	hs := map[string]string{}
	for _, h := range d.Headers {
		hs[h.Key] = string(h.Value)
	}
	if hs[HeaderSourcePartition] != "3" || hs[HeaderSourceOffset] != "42" || hs[HeaderSourceGroup] != "billing" || hs[HeaderSourceTopic] != "orders" {
		t.Fatalf("dlq headers = %v", hs)
	}
}

func TestPoisonBatchGoesToDLQAfterMaxAttempts(t *testing.T) {
	fc := newFakeClient(fetchOf("orders", span{partition: 0, from: 0, count: 2}))
	var mu sync.Mutex
	calls := 0
	dlq := &recordingDLQ{}
	c, _ := newBatchConsumer(fc, "g", func(context.Context, []Record) error {
		mu.Lock()
		defer mu.Unlock()
		calls++
		return errors.New("always fails")
	}, dlq, fastBatchOpts())
	_ = runUntil(t, c.Run, func() { fc.waitForEvent(t, "commit", 1) })

	mu.Lock()
	defer mu.Unlock()
	if calls != 3 {
		t.Fatalf("handler calls = %d, want MaxAttempts (3)", calls)
	}
	if n := len(dlq.records()); n != 2 {
		t.Fatalf("dlq records = %d, want the 2 records of the batch", n)
	}
	_, commits := fc.snapshot()
	if len(commits) != 1 || commits[0][0].Offset != 1 {
		t.Fatalf("commits = %v, want one commit at the batch end", commitOffsets(commits))
	}
}

func TestBatchDefaultMaxAttempts(t *testing.T) {
	c, err := newBatchConsumer(newFakeClient(), "g", func(context.Context, []Record) error { return nil }, nopDLQ{}, BatchOptions{DLQTopic: "d"})
	if err != nil {
		t.Fatal(err)
	}
	if c.opts.MaxAttempts != 5 {
		t.Fatalf("default MaxAttempts = %d, want 5", c.opts.MaxAttempts)
	}
}

func TestConstructorsRejectInvalidArguments(t *testing.T) {
	fc := newFakeClient()
	h := func(context.Context, Record) error { return nil }
	bh := func(context.Context, []Record) error { return nil }
	if _, err := newConsumer(fc, "g", h, nopDLQ{}, Options{}); err == nil {
		t.Error("NewConsumer accepted an empty DLQ topic")
	}
	if _, err := newBatchConsumer(fc, "g", bh, nopDLQ{}, BatchOptions{}); err == nil {
		t.Error("NewBatchConsumer accepted an empty DLQ topic")
	}
	if _, err := newConsumer(fc, "g", nil, nopDLQ{}, Options{DLQTopic: "d"}); err == nil {
		t.Error("NewConsumer accepted a nil handler")
	}
	if _, err := newBatchConsumer(fc, "g", bh, nil, BatchOptions{DLQTopic: "d"}); err == nil {
		t.Error("NewBatchConsumer accepted a nil DLQ producer")
	}
	if _, err := NewConsumer(nil, h, nopDLQ{}, Options{DLQTopic: "d"}); err == nil {
		t.Error("NewConsumer accepted a nil client")
	}
	if _, err := NewBatchConsumer(nil, bh, nopDLQ{}, BatchOptions{DLQTopic: "d"}); err == nil {
		t.Error("NewBatchConsumer accepted a nil client")
	}
}
