package bootstrap

import (
	"bytes"
	"context"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buidangphuc/team-identity/internal/config"
	"github.com/buidangphuc/team-identity/internal/repository"
)

// blockingRepo implements only the purge calls; any other method panics (nil embed).
type blockingRepo struct {
	repository.SessionRepository
	started  chan struct{}
	once     sync.Once
	calls    atomic.Int32
	finished atomic.Bool
}

func (r *blockingRepo) PurgeSessions(ctx context.Context, _ time.Time, _ time.Duration, _ int) (int, error) {
	r.calls.Add(1)
	r.once.Do(func() { close(r.started) })
	<-ctx.Done() // an in-flight sweep that only ends on shutdown
	r.finished.Store(true)
	return 0, ctx.Err()
}

func (r *blockingRepo) PurgeLoginHistory(ctx context.Context, _ time.Time, _ int) (int, error) {
	r.calls.Add(1)
	return 0, nil
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *syncBuf) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func settings(enabled bool) *config.Settings {
	return &config.Settings{Retention: config.Retention{
		Session:        time.Hour,
		LoginHistory:   2 * time.Hour,
		SweepInterval:  time.Millisecond,
		SweepBatchSize: 500,
		SweepEnabled:   enabled,
	}}
}

// Scenario: Session retention: shutdown drains the sweeper
func TestCloseResourcesStopsSweeper(t *testing.T) {
	repo := &blockingRepo{started: make(chan struct{})}
	buf := &syncBuf{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	res := &Resources{} // Pool nil: sweeper must not depend on Kafka or a pool here

	res.startSessionSweeper(settings(true), repo, logger)
	select {
	case <-repo.started:
	case <-time.After(5 * time.Second):
		t.Fatal("sweeper never started a sweep (independent of Kafka)")
	}

	if err := CloseResources(context.Background(), res); err != nil {
		t.Fatalf("CloseResources: %v", err)
	}
	if !repo.finished.Load() {
		t.Fatal("CloseResources returned before the in-flight sweep drained")
	}
	if err := CloseResources(context.Background(), res); err != nil {
		t.Fatalf("second CloseResources must be a no-op: %v", err)
	}
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.Contains(line, "level=ERROR") || strings.Contains(line, "level=WARN") {
			t.Fatalf("shutdown must not log error/warn, got: %s", line)
		}
	}
}

// Scenario: Session retention: disabled sweeper
func TestSweeperDisabled(t *testing.T) {
	repo := &blockingRepo{started: make(chan struct{})}
	res := &Resources{}
	before := runtime.NumGoroutine()

	res.startSessionSweeper(settings(false), repo, slog.New(slog.NewTextHandler(&syncBuf{}, nil)))

	if after := runtime.NumGoroutine(); after != before {
		t.Fatalf("disabled sweeper started a goroutine: %d -> %d", before, after)
	}
	if err := CloseResources(context.Background(), res); err != nil {
		t.Fatalf("CloseResources: %v", err)
	}
	if n := repo.calls.Load(); n != 0 {
		t.Fatalf("disabled sweeper purged %d times", n)
	}
}
