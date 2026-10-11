package tombstone_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/buidangphuc/team-search/internal/tombstone"
)

type fakePurger struct {
	mu      sync.Mutex
	cutoffs []time.Time
	errs    []error
	done    chan struct{}
}

func (f *fakePurger) PurgeTombstones(_ context.Context, olderThan time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cutoffs = append(f.cutoffs, olderThan)
	var err error
	if len(f.errs) > 0 {
		err, f.errs = f.errs[0], f.errs[1:]
	}
	f.done <- struct{}{}
	if err != nil {
		return 0, err
	}
	return 3, nil
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

// D6: each tick purges tombstones older than now - TTL, logs the count, and a
// failure is logged and retried on the next tick, not fatal.
func TestLoop_PurgesEveryIntervalWithTTLCutoff(t *testing.T) {
	clock := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	ticks := make(chan time.Time)
	var gotInterval []time.Duration
	var mu sync.Mutex
	p := &fakePurger{done: make(chan struct{}), errs: []error{nil, errors.New("cluster down"), nil}}
	logs := &syncBuf{}
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		tombstone.Loop{
			Purger: p, TTL: 30 * time.Second, Interval: 2 * time.Second,
			Logger: slog.New(slog.NewTextHandler(logs, nil)),
			Now:    func() time.Time { mu.Lock(); defer mu.Unlock(); return clock },
			After: func(d time.Duration) <-chan time.Time {
				mu.Lock()
				gotInterval = append(gotInterval, d)
				mu.Unlock()
				return ticks
			},
		}.Run(ctx)
		close(stopped)
	}()

	for i := 0; i < 3; i++ {
		mu.Lock()
		clock = clock.Add(2 * time.Second)
		mu.Unlock()
		ticks <- clock
		<-p.done
	}
	cancel()
	<-stopped

	if len(p.cutoffs) != 3 {
		t.Fatalf("want 3 purges, got %d", len(p.cutoffs))
	}
	base := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	for i, c := range p.cutoffs {
		want := base.Add(time.Duration(i+1)*2*time.Second - 30*time.Second)
		if !c.Equal(want) {
			t.Errorf("purge %d cutoff = %s, want %s", i, c, want)
		}
	}
	for _, d := range gotInterval {
		if d != 2*time.Second {
			t.Errorf("waited %s, want the 2s interval", d)
		}
	}
	out := logs.String()
	if strings.Count(out, "purged=3") != 2 || !strings.Contains(out, "cluster down") {
		t.Errorf("want two purge counts and the failure logged, got:\n%s", out)
	}
}
