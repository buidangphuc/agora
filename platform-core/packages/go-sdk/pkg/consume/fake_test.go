package consume

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

type nopDLQ struct{}

func (nopDLQ) PublishRecord(context.Context, *kgo.Record) error { return nil }

type recordingDLQ struct {
	mu   sync.Mutex
	recs []*kgo.Record
	err  error
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

// fakeClient serves scripted fetches and models the BlockRebalanceOnPoll gate: a
// revoke requested while a fetch is being processed only takes effect in
// AllowRebalance. Every step is appended to events.
type fakeClient struct {
	mu      sync.Mutex
	script  []kgo.Fetches
	events  []string
	commits [][]*kgo.Record
	revoke  bool  // a rebalance wants to revoke; applied by AllowRebalance
	polls   []int // the max records asked of each poll
	// commitErrs are returned, in order, by the first CommitRecords calls.
	commitErrs []error
	changed    chan struct{}
}

func newFakeClient(fetches ...kgo.Fetches) *fakeClient {
	return &fakeClient{script: fetches, changed: make(chan struct{}, 64)}
}

func (f *fakeClient) event(e string) {
	f.events = append(f.events, e)
	select {
	case f.changed <- struct{}{}:
	default:
	}
}

func (f *fakeClient) PollRecords(ctx context.Context, maxRecords int) kgo.Fetches {
	f.mu.Lock()
	f.polls = append(f.polls, maxRecords)
	if len(f.script) > 0 {
		next := f.script[0]
		f.script = f.script[1:]
		f.event("poll")
		f.mu.Unlock()
		return next
	}
	f.mu.Unlock()
	<-ctx.Done()
	return nil
}

func (f *fakeClient) CommitRecords(_ context.Context, rs ...*kgo.Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commits = append(f.commits, append([]*kgo.Record(nil), rs...))
	f.event("commit")
	if len(f.commitErrs) > 0 {
		err := f.commitErrs[0]
		f.commitErrs = f.commitErrs[1:]
		return err
	}
	return nil
}

func (f *fakeClient) AllowRebalance() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.revoke {
		f.revoke = false
		f.event("revoke")
	}
	f.event("allow")
}

// requestRevoke is what the broker does mid-fetch: it asks for a revoke, which
// the gate holds back until AllowRebalance.
func (f *fakeClient) requestRevoke() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revoke = true
}

// maxPolls returns the max records asked of each poll so far.
func (f *fakeClient) maxPolls() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int(nil), f.polls...)
}

func (f *fakeClient) snapshot() (events []string, commits [][]*kgo.Record) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.events...), append([][]*kgo.Record(nil), f.commits...)
}

// waitForEvent blocks until n events named name were seen.
func (f *fakeClient) waitForEvent(t *testing.T, name string, n int) {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		evs, _ := f.snapshot()
		c := 0
		for _, e := range evs {
			if e == name {
				c++
			}
		}
		if c >= n {
			return
		}
		select {
		case <-f.changed:
		case <-timeout:
			t.Fatalf("event %q x%d not seen; events = %v", name, n, evs)
		}
	}
}

// partFetch builds a fetch of one topic holding records of the given partitions;
// each entry is partition -> first offset, with count records.
func fetchOf(topic string, spans ...span) kgo.Fetches {
	var parts []kgo.FetchPartition
	for _, s := range spans {
		var recs []*kgo.Record
		for i := range s.count {
			off := s.from + int64(i)
			recs = append(recs, &kgo.Record{
				Topic: topic, Partition: s.partition, Offset: off,
				Key: []byte("k"), Value: []byte(fmt.Sprintf("p%d-o%d", s.partition, off)),
				Headers: s.headers,
			})
		}
		parts = append(parts, kgo.FetchPartition{Partition: s.partition, Records: recs})
	}
	return kgo.Fetches{{Topics: []kgo.FetchTopic{{Topic: topic, Partitions: parts}}}}
}

type span struct {
	partition int32
	from      int64
	count     int
	headers   []kgo.RecordHeader
}

// runUntil runs fn(ctx) and cancels it once cond is true, returning fn's error.
func runUntil(t *testing.T, run func(context.Context) error, cond func()) error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- run(ctx) }()
	cond()
	cancel()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after cancel")
		return nil
	}
}
