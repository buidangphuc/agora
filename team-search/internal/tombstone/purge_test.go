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

func (s *syncBuf) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

// D6: each tick purges tombstones older than now - TTL, logs the count, and a
// failure is logged and retried on the next tick, not fatal.
func TestLoop_PurgesEveryIntervalWithTTLCutoff(t *testing.T) {
	// Each clock reading advances 2s, so the Nth purge computes now = base + N*2s.
	base := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	clock := base
	now := func() time.Time { mu.Lock(); defer mu.Unlock(); clock = clock.Add(2 * time.Second); return clock }
	p := &fakePurger{done: make(chan struct{}, 8), errs: []error{nil, errors.New("cluster down"), nil}}
	logs := &syncBuf{}
	comp, err := tombstone.Loop{
		Purger: p, TTL: 30 * time.Second, Interval: 5 * time.Millisecond,
		Logger: slog.New(slog.NewTextHandler(logs, nil)), Now: now,
	}.Component()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() { _ = comp.Run(ctx); close(stopped) }()

	for i := 0; i < 3; i++ {
		<-p.done
	}
	cancel()
	<-stopped

	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.cutoffs) < 3 {
		t.Fatalf("want at least 3 purges, got %d", len(p.cutoffs))
	}
	for i, c := range p.cutoffs {
		want := base.Add(time.Duration(i+1)*2*time.Second - 30*time.Second)
		if !c.Equal(want) {
			t.Errorf("purge %d cutoff = %s, want %s", i, c, want)
		}
	}
	out := logs.String()
	if strings.Count(out, "purged=3") < 2 || !strings.Contains(out, "cluster down") {
		t.Errorf("want the purge counts and the failure logged, got:\n%s", out)
	}
}

func TestLoop_RejectsNonPositiveInterval(t *testing.T) {
	if _, err := (tombstone.Loop{Purger: &fakePurger{}, TTL: time.Hour, Logger: slog.Default()}).Component(); err == nil {
		t.Fatal("a zero interval must be rejected")
	}
}
